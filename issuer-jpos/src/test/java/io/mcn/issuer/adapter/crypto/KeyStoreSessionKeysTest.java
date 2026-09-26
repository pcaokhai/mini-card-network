package io.mcn.issuer.adapter.crypto;

import static org.assertj.core.api.Assertions.assertThat;

import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRow;
import io.mcn.issuer.adapter.persistence.TestDataSources;
import java.util.HexFormat;
import javax.sql.DataSource;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class KeyStoreSessionKeysTest {
  private static final String LMK_HEX =
      "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
  private static final byte[] K1 = HexFormat.of().parseHex("11".repeat(16));
  private static final byte[] K2 = HexFormat.of().parseHex("22".repeat(16));
  private static final byte[] K3 = HexFormat.of().parseHex("33".repeat(16));

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  private final JCESecurityModule hsm = new JCESecurityModule(LMK_HEX);
  private DataSource ds;
  private KeyStoreRepository repo;
  private KeyStoreSessionKeys keys;

  @BeforeEach
  void setUp() throws Exception {
    ds = TestDataSources.migrated(postgres);
    try (var conn = ds.getConnection()) {
      conn.createStatement().execute("DELETE FROM key_store");
    }
    repo = new KeyStoreRepository(ds);
    keys = new KeyStoreSessionKeys(repo, hsm, "970499");
  }

  private long insertAndActivate(byte[] clear) {
    long id =
        repo.insert(
            new KeyStoreRow(
                0,
                "ZAK",
                "970499",
                HexFormat.of().formatHex(hsm.wrapUnderLmk(clear)),
                hsm.computeKcv(clear),
                "PENDING",
                null,
                null,
                null));
    repo.activate(id);
    return id;
  }

  private void execute(String sql) throws Exception {
    try (var conn = ds.getConnection()) {
      conn.createStatement().execute(sql);
    }
  }

  @Test
  @DisplayName("SEC-G15: startup seeds the env key once, stored only as a cryptogram under the LMK")
  void should_seedOnce_andStoreOnlyTheCryptogram() {
    keys.ensureActive("ZAK", K1.clone());
    keys.ensureActive("ZAK", K2.clone()); // a second bean starting: an ACTIVE key already exists

    assertThat(repo.findAll()).hasSize(1);
    KeyStoreRow row = repo.findAll().get(0);
    assertThat(row.status()).isEqualTo("ACTIVE");
    assertThat(row.activatedAt()).isNotNull();
    assertThat(row.keyUnderLmkHex()).doesNotContainIgnoringCase(HexFormat.of().formatHex(K1));
    assertThat(keys.active("ZAK")).isEqualTo(K1);
  }

  @Test
  @DisplayName("SEC-G15: after a key change the ACTIVE key is the new one, the old one retired")
  void should_serveTheNewKey_andTheOldAsRecentlyRetired_afterAKeyChange() {
    keys.ensureActive("ZAK", K1.clone());
    insertAndActivate(K2);

    assertThat(keys.active("ZAK")).isEqualTo(K2);
    assertThat(keys.recentlyRetired("ZAK")).hasValueSatisfying(k -> assertThat(k).isEqualTo(K1));
  }

  @Test
  @DisplayName("SEC-G15: the retired key stops being accepted after the 5-minute window")
  void should_dropTheRetiredKey_afterTheDualKeyWindow() throws Exception {
    keys.ensureActive("ZAK", K1.clone());
    insertAndActivate(K2);
    execute(
        "UPDATE key_store SET retired_at = now() - interval '6 minutes' WHERE status = 'RETIRED'");

    assertThat(keys.recentlyRetired("ZAK")).isEmpty();
  }

  @Test
  @DisplayName("SEC-G15: a key that was never activated is never accepted, even if RETIRED")
  void should_neverServeAKeyThatWasNeverActivated() throws Exception {
    keys.ensureActive("ZAK", K1.clone());
    execute(
        "INSERT INTO key_store (key_type, counterparty, key_under_lmk, kcv, status, retired_at)"
            + " VALUES ('ZAK', '970499', '"
            + HexFormat.of().formatHex(hsm.wrapUnderLmk(K3))
            + "', '"
            + hsm.computeKcv(K3)
            + "', 'RETIRED', now())");

    assertThat(keys.active("ZAK")).isEqualTo(K1);
    assertThat(keys.recentlyRetired("ZAK")).isEmpty();
  }

  @Test
  @DisplayName("SEC-G15: every call hands out a fresh copy, so zeroing it never damages the next")
  void should_returnAFreshCopy_thatTheCallerMayZero() {
    keys.ensureActive("ZAK", K1.clone());
    byte[] first = keys.active("ZAK");
    java.util.Arrays.fill(first, (byte) 0);

    assertThat(keys.active("ZAK")).isEqualTo(K1);
  }
}
