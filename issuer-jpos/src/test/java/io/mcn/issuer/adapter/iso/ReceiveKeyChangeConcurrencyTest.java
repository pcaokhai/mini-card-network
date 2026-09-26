package io.mcn.issuer.adapter.iso;

import static org.assertj.core.api.Assertions.assertThat;

import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.persistence.TestDataSources;
import java.security.SecureRandom;
import java.util.ArrayList;
import java.util.HexFormat;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import javax.sql.DataSource;
import org.jpos.iso.ISOMsg;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * SF2 (#122 review): the gateway resends an unanswered 0800/161 (#121), so two copies of one key
 * change can be processed at once. Both must answer 00 and leave exactly one ACTIVE row.
 */
@Testcontainers
class ReceiveKeyChangeConcurrencyTest {
  private static final String LMK_HEX =
      "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
  private static final byte[] ZMK = new byte[16];
  private static final int ROUNDS = 20;

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  @DisplayName("SF2: two concurrent copies of one key change both answer 00, one ACTIVE row")
  void should_acknowledgeBoth_when_theSameKeyChangeArrivesTwiceAtOnce() throws Exception {
    DataSource ds = TestDataSources.migrated(postgres);
    var keyStore = new KeyStoreRepository(ds);
    var receiveKeyChange =
        new ReceiveKeyChange(
            new JCESecurityModule(LMK_HEX), keyStore, new AuditLogRepository(ds), ZMK, "970499");
    var random = new SecureRandom();

    try (var pool = Executors.newFixedThreadPool(2)) {
      for (int round = 0; round < ROUNDS; round++) {
        byte[] key = new byte[16];
        random.nextBytes(key);
        String de48 = "ZAK:" + HexFormat.of().formatHex(underZmk(key));
        var start = new CountDownLatch(1);
        List<Future<Boolean>> results = new ArrayList<>();
        for (int copy = 0; copy < 2; copy++) {
          results.add(
              pool.submit(
                  () -> {
                    ISOMsg request = new ISOMsg("0800");
                    request.set(70, "161");
                    request.set(48, de48);
                    start.await();
                    return receiveKeyChange.receive(request);
                  }));
        }
        start.countDown();
        for (var result : results) {
          assertThat(result.get(30, TimeUnit.SECONDS)).as("round %d", round).isTrue();
        }
        assertThat(count(ds, "SELECT count(*) FROM key_store WHERE status = 'ACTIVE'"))
            .as("round %d", round)
            .isEqualTo(1);
        String kcv = new JCESecurityModule(LMK_HEX).computeKcv(key);
        assertThat(
                count(
                    ds,
                    "SELECT count(*) FROM key_store WHERE status = 'RETIRED' AND kcv = '"
                        + kcv
                        + "'"))
            .as("round %d: this round's key must not also be the recently retired one", round)
            .isZero();
      }
    }
    assertThat(
            count(ds, "SELECT count(*) FROM audit_log WHERE action = 'key_change.replay_rejected'"))
        .isZero();
    assertThat(count(ds, "SELECT count(*) FROM audit_log WHERE action = 'key_change.activated'"))
        .isEqualTo(ROUNDS); // one per key change, not one per copy
  }

  private static byte[] underZmk(byte[] clearKey) throws Exception {
    byte[] nonce = new byte[12];
    new SecureRandom().nextBytes(nonce);
    Cipher gcm = Cipher.getInstance("AES/GCM/NoPadding");
    gcm.init(Cipher.ENCRYPT_MODE, new SecretKeySpec(ZMK, "AES"), new GCMParameterSpec(128, nonce));
    byte[] ciphertext = gcm.doFinal(clearKey);
    byte[] cryptogram = new byte[nonce.length + ciphertext.length];
    System.arraycopy(nonce, 0, cryptogram, 0, nonce.length);
    System.arraycopy(ciphertext, 0, cryptogram, nonce.length, ciphertext.length);
    return cryptogram;
  }

  private static long count(DataSource ds, String sql) throws Exception {
    try (var conn = ds.getConnection();
        var rs = conn.createStatement().executeQuery(sql)) {
      rs.next();
      return rs.getLong(1);
    }
  }
}
