package io.mcn.issuer.adapter.iso;

import static org.assertj.core.api.Assertions.assertThat;

import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import io.mcn.issuer.adapter.crypto.KeyStoreSessionKeys;
import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.persistence.TestDataSources;
import java.security.SecureRandom;
import java.sql.Connection;
import java.util.HexFormat;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import javax.sql.DataSource;
import org.jpos.iso.ISOMsg;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * SEC-G23: a key's activation and its {@code key_change.activated} audit row commit together, so
 * the 0810 answer always matches {@code key_store}: 00 with the new key ACTIVE and audited, or 96
 * with nothing changed - never 96 while the new key is already live.
 */
@Testcontainers
class ReceiveKeyChangeAtomicityTest {
  private static final String LMK_HEX =
      "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
  private static final byte[] ZMK = new byte[16];
  private static final byte[] OLD_KEY = HexFormat.of().parseHex("11".repeat(16));

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  private final JCESecurityModule hsm = new JCESecurityModule(LMK_HEX);
  private DataSource ds;
  private KeyStoreRepository keyStore;

  @BeforeEach
  void seedTheOldKey() throws Exception {
    ds = TestDataSources.migrated(postgres);
    execute("DELETE FROM key_store");
    execute("DELETE FROM audit_log WHERE entity_type = 'key_store'");
    keyStore = new KeyStoreRepository(ds);
    new KeyStoreSessionKeys(keyStore, hsm, "970499").ensureActive("ZAK", OLD_KEY.clone());
  }

  @Test
  @DisplayName(
      "SEC-G23: if the audit write fails, the activation rolls back with it - 96 and the old key"
          + " still ACTIVE")
  void should_rollBackTheActivation_when_itsAuditWriteFails() throws Exception {
    var failingAudit =
        new AuditLogRepository(ds) {
          @Override
          public long record(
              Connection conn,
              String actor,
              String action,
              String entityType,
              String entityId,
              String before,
              String after) {
            throw new IllegalStateException("injected: audit_log insert failed");
          }

          @Override
          public long record(
              String actor,
              String action,
              String entityType,
              String entityId,
              String before,
              String after) {
            throw new IllegalStateException("injected: audit_log insert failed");
          }
        };
    byte[] newKey = randomKey();

    boolean answered00 =
        new ReceiveKeyChange(hsm, keyStore, failingAudit, ZMK, "970499").receive(keyChange(newKey));

    assertThat(answered00).isFalse();
    assertThat(activeKey()).isEqualTo(OLD_KEY);
    assertThat(count("SELECT count(*) FROM key_store WHERE status = 'RETIRED'")).isZero();
  }

  @Test
  @DisplayName("SEC-G23: a successful key change is ACTIVE and audited, in one commit")
  void should_activateAndAudit_when_everythingSucceeds() throws Exception {
    byte[] newKey = randomKey();

    boolean answered00 =
        new ReceiveKeyChange(hsm, keyStore, new AuditLogRepository(ds), ZMK, "970499")
            .receive(keyChange(newKey));

    assertThat(answered00).isTrue();
    assertThat(activeKey()).isEqualTo(newKey);
    assertThat(count("SELECT count(*) FROM audit_log WHERE action = 'key_change.activated'"))
        .isEqualTo(1);
  }

  private byte[] activeKey() {
    return new KeyStoreSessionKeys(keyStore, hsm, "970499").active("ZAK");
  }

  private static byte[] randomKey() {
    byte[] key = new byte[16];
    new SecureRandom().nextBytes(key);
    return key;
  }

  private static ISOMsg keyChange(byte[] clearKey) throws Exception {
    byte[] nonce = new byte[12];
    new SecureRandom().nextBytes(nonce);
    Cipher gcm = Cipher.getInstance("AES/GCM/NoPadding");
    gcm.init(Cipher.ENCRYPT_MODE, new SecretKeySpec(ZMK, "AES"), new GCMParameterSpec(128, nonce));
    byte[] ciphertext = gcm.doFinal(clearKey);
    byte[] cryptogram = new byte[nonce.length + ciphertext.length];
    System.arraycopy(nonce, 0, cryptogram, 0, nonce.length);
    System.arraycopy(ciphertext, 0, cryptogram, nonce.length, ciphertext.length);
    ISOMsg request = new ISOMsg("0800");
    request.set(70, "161");
    request.set(48, "ZAK:" + HexFormat.of().formatHex(cryptogram));
    return request;
  }

  private void execute(String sql) throws Exception {
    try (var conn = ds.getConnection()) {
      conn.createStatement().execute(sql);
    }
  }

  private long count(String sql) throws Exception {
    try (var conn = ds.getConnection();
        var rs = conn.createStatement().executeQuery(sql)) {
      rs.next();
      return rs.getLong(1);
    }
  }
}
