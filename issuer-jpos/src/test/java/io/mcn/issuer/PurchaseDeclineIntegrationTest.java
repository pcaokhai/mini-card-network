package io.mcn.issuer;

import static org.assertj.core.api.Assertions.assertThat;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.crypto.CardCrypto;
import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import io.mcn.issuer.adapter.seed.SeedLoader;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.file.Path;
import java.sql.ResultSet;
import java.util.HexFormat;
import java.util.Map;
import javax.crypto.Cipher;
import javax.crypto.spec.SecretKeySpec;
import javax.sql.DataSource;
import org.flywaydb.core.Flyway;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.packager.GenericPackager;
import org.jpos.q2.Q2;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.MethodOrderer;
import org.junit.jupiter.api.Order;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestMethodOrder;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * Drives the real {@code src/dist/deploy} descriptors under a single real embedded {@link Q2}
 * (QServer + both ISORequestListeners + the authorization TransactionManager) over a raw socket -
 * proving the full chain wiring, not just individual participants. One shared Q2/link for the whole
 * class (not per-test): repeatedly starting/shutting down embedded Q2 instances in the same JVM was
 * flaky (jPOS's Q2/NameRegistrar carry process-wide static state). Tests are ordered so the
 * unsigned-link case runs before the sign-on that every other test needs.
 */
@Testcontainers
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class PurchaseDeclineIntegrationTest {

  private static final String ENCRYPTION_KEY_HEX = "0".repeat(64);
  private static final String HMAC_KEY_HEX = "1".repeat(64);
  private static final String LMK_HEX =
      "00112233445566778899aabbccddeeff00112233445566778899aabbccddee";
  private static final byte[] ZAK = HexFormat.of().parseHex("3132333435363738393a3b3c3d3e3f40");
  private static final byte[] ZPK = HexFormat.of().parseHex("2132333435363738393a3b3c3d3e3f41");
  private static final String FIXTURE_PIN =
      "1234"; // every contracts/fixtures/cards.json card shares this PIN

  @Container
  static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine");

  private static Q2 q2;
  private static int port;
  private static GenericPackager packager;
  private static DataSource dataSource;

  private static int freePort() throws Exception {
    try (ServerSocket s = new ServerSocket(0)) {
      return s.getLocalPort();
    }
  }

  @BeforeAll
  static void start() throws Exception {
    HikariConfig cfg = new HikariConfig();
    cfg.setJdbcUrl(postgres.getJdbcUrl());
    cfg.setUsername(postgres.getUsername());
    cfg.setPassword(postgres.getPassword());
    dataSource = new HikariDataSource(cfg);
    Flyway.configure().dataSource(dataSource).load().migrate();
    new SeedLoader()
        .load(
            dataSource,
            new CardCrypto(ENCRYPTION_KEY_HEX, HMAC_KEY_HEX),
            Path.of("../contracts/fixtures/cards.json"));

    port = freePort();
    System.setProperty("DATABASE_URL", postgres.getJdbcUrl());
    System.setProperty("DB_USER", postgres.getUsername());
    System.setProperty("DB_PASSWORD", postgres.getPassword());
    System.setProperty("ISO_PORT", String.valueOf(port));
    System.setProperty(
        "ISO_PACKAGER_CFG", Path.of("src/dist/cfg/iso87ascii.xml").toAbsolutePath().toString());
    System.setProperty("PAN_ENCRYPTION_KEY_HEX", ENCRYPTION_KEY_HEX);
    System.setProperty("PAN_HMAC_KEY_HEX", HMAC_KEY_HEX);
    System.setProperty("LMK_TEST_VALUE_HEX", LMK_HEX);
    System.setProperty("ZAK_HEX", HexFormat.of().formatHex(ZAK));
    System.setProperty("ZPK_HEX", HexFormat.of().formatHex(ZPK));
    System.setProperty("ZMK_HEX", "00".repeat(16));

    packager = new GenericPackager("src/dist/cfg/iso87ascii.xml");

    q2 = new Q2(Path.of("src/dist/deploy").toAbsolutePath().toString());
    q2.start();
    if (!q2.ready(10_000)) {
      throw new IllegalStateException("Q2 did not become ready in time");
    }
    Thread.sleep(300); // let QServer's accept loop bind before the first test connects
  }

  @AfterAll
  static void stop() {
    q2.shutdown(true);
  }

  private static byte[] send(byte[] body) throws Exception {
    try (Socket socket = new Socket("127.0.0.1", port)) {
      socket.setSoTimeout(5000);
      var out = new java.io.DataOutputStream(socket.getOutputStream());
      out.writeShort(body.length);
      out.write(body);
      var in = new java.io.DataInputStream(socket.getInputStream());
      int len = in.readUnsignedShort();
      byte[] resp = new byte[len];
      in.readFully(resp);
      return resp;
    }
  }

  private static ISOMsg buildIso(String mti, Map<Integer, String> fields) throws ISOException {
    ISOMsg msg = new ISOMsg();
    msg.setPackager(packager);
    msg.setMTI(mti);
    for (var entry : fields.entrySet()) {
      msg.set(entry.getKey(), entry.getValue());
    }
    return msg;
  }

  private static ISOMsg unpack(byte[] bytes) throws ISOException {
    ISOMsg msg = new ISOMsg();
    msg.setPackager(packager);
    msg.unpack(bytes);
    return msg;
  }

  private static void signOn(int stan) throws Exception {
    ISOMsg signOn = buildIso("0800", Map.of(7, "0922120000", 11, pad(stan), 70, "001"));
    send(signOn.pack());
  }

  private static String pad(int stan) {
    return String.format("%06d", stan);
  }

  private static ISOMsg purchase(int stan, String pan, long amountMinor) throws Exception {
    ISOMsg request =
        buildIso(
            "0200",
            Map.of(
                3, "000000",
                4, String.format("%012d", amountMinor),
                7, "0922120000",
                11, pad(stan),
                37, "GOCPHO" + pad(stan),
                41, "00000042",
                42, "GOCPHO000000001"));
    request.set(2, pan);
    // VerifySecurity (MCN-503) verifies DE 52/64 on every 0200; a real gateway always sends
    // both, so the test builds a genuine PIN block (fixture PIN) and Retail MAC too - otherwise
    // every purchase would fail RC 96 before CheckCard's own RC 14/62/51 checks get exercised.
    request.set(52, HexFormat.of().formatHex(encryptPinBlock(buildPinBlock(FIXTURE_PIN, pan))));
    JCESecurityModule securityModule = new JCESecurityModule(LMK_HEX);
    byte[] mac = securityModule.computeMac(request.pack(), ZAK);
    request.set(64, HexFormat.of().formatHex(mac));
    return unpack(send(request.pack()));
  }

  /** ISO 9564-1 format 0, matching web-next's {@code buildPinBlock} (pinblock.ts). */
  private static byte[] buildPinBlock(String pin, String pan) {
    String pinField = "0" + Integer.toHexString(pin.length()) + pin;
    StringBuilder padded = new StringBuilder(pinField);
    while (padded.length() < 16) {
      padded.append('F');
    }
    String panDigits = pan.substring(0, pan.length() - 1);
    panDigits = panDigits.substring(panDigits.length() - 12);
    String panField = "0000" + panDigits;

    byte[] result = new byte[8];
    for (int i = 0; i < 8; i++) {
      int hi =
          Character.digit(padded.charAt(i * 2), 16) ^ Character.digit(panField.charAt(i * 2), 16);
      int lo =
          Character.digit(padded.charAt(i * 2 + 1), 16)
              ^ Character.digit(panField.charAt(i * 2 + 1), 16);
      result[i] = (byte) ((hi << 4) | lo);
    }
    return result;
  }

  /** Test-only counterpart to {@code JCESecurityModule.decryptPinBlock}, same key expansion. */
  private static byte[] encryptPinBlock(byte[] clearBlock) throws Exception {
    byte[] tripleKey = new byte[24];
    System.arraycopy(ZPK, 0, tripleKey, 0, 16);
    System.arraycopy(ZPK, 0, tripleKey, 16, 8);
    Cipher cipher = Cipher.getInstance("DESede/ECB/NoPadding");
    cipher.init(Cipher.ENCRYPT_MODE, new SecretKeySpec(tripleKey, "DESede"));
    return cipher.doFinal(clearBlock);
  }

  @Test
  @Order(1)
  void purchaseOnUnsignedLinkIsDeclinedRc91() throws Exception {
    ISOMsg response = purchase(1, "9704360000004417", 10000L);

    assertThat(response.getMTI()).isEqualTo("0210");
    assertThat(response.getString(39)).isEqualTo("91");
  }

  @Test
  @Order(2)
  void purchaseWithUnknownCardIsDeclinedRc14() throws Exception {
    signOn(2);
    ISOMsg response = purchase(3, "9704360000009999", 10000L);

    assertThat(response.getString(39)).isEqualTo("14");
  }

  @Test
  @Order(3)
  void purchaseWithBlockedCardIsDeclinedRc62() throws Exception {
    ISOMsg response = purchase(4, "9704360000003310", 10000L);

    assertThat(response.getString(39)).isEqualTo("62");
  }

  @Test
  @Order(4)
  void duplicateRequestReplaysStoredResponse() throws Exception {
    ISOMsg first = purchase(5, "9704360000009021", 10000L);
    ISOMsg second = purchase(5, "9704360000009021", 10000L);

    assertThat(second.getString(39)).isEqualTo(first.getString(39));
  }

  @Test
  @Order(5)
  void sufficientFundsIsReallyApprovedWithAuthCode() throws Exception {
    // MCN-302b replaces the RC-96 placeholder with a real approval and a balanced double-entry
    // journal (journal_entry + two ledger_posting rows), asserted directly against the DB here.
    long accountId = accountIdByAccountNo("ACC-crd_normal0001");
    long before = accountBalance(accountId);
    long journalsBefore = balancedJournalCount(accountId);

    ISOMsg response = purchase(6, "9704360000004417", 10000L);

    assertThat(response.getString(39)).isEqualTo("00");
    assertThat(response.getString(38)).hasSize(6);
    assertThat(accountBalance(accountId)).isEqualTo(before - 10000L);
    assertThat(balancedJournalCount(accountId)).isEqualTo(journalsBefore + 1);
  }

  @Test
  @Order(6)
  void purchaseExceedingBalanceIsDeclinedRc51WithoutPostingLedger() throws Exception {
    // tok_low (ACC-crd_lowbal0002) seeded at 80,000 minor units, already debited 10,000 by
    // duplicateRequestReplaysStoredResponse (Order 4) - request well above what remains.
    long accountId = accountIdByAccountNo("ACC-crd_lowbal0002");
    long before = accountBalance(accountId);
    long journalsBefore = balancedJournalCount(accountId);

    ISOMsg response = purchase(7, "9704360000009021", 500000L);

    assertThat(response.getString(39)).isEqualTo("51");
    assertThat(accountBalance(accountId)).isEqualTo(before);
    assertThat(balancedJournalCount(accountId)).isEqualTo(journalsBefore);
  }

  private static long accountIdByAccountNo(String accountNo) throws Exception {
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement("SELECT id FROM account WHERE account_no = ?")) {
      stmt.setString(1, accountNo);
      try (ResultSet rs = stmt.executeQuery()) {
        rs.next();
        return rs.getLong(1);
      }
    }
  }

  private static long accountBalance(long accountId) throws Exception {
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement("SELECT available_balance FROM account WHERE id = ?")) {
      stmt.setLong(1, accountId);
      try (ResultSet rs = stmt.executeQuery()) {
        rs.next();
        return rs.getLong(1);
      }
    }
  }

  private static long balancedJournalCount(long accountId) throws Exception {
    try (var conn = dataSource.getConnection();
        var stmt =
            conn.prepareStatement(
                """
                SELECT count(DISTINCT lp.journal_id)
                FROM ledger_posting lp
                WHERE lp.account_id = ? AND lp.direction = 'D'""")) {
      stmt.setLong(1, accountId);
      try (ResultSet rs = stmt.executeQuery()) {
        rs.next();
        return rs.getLong(1);
      }
    }
  }
}
