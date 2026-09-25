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
import org.junit.jupiter.api.DisplayName;
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
    return financial(stan, pan, "000000", amountMinor);
  }

  /** A 0200 with {@code processingCode}; {@code amountMinor} null leaves DE 4 out (inquiry). */
  private static ISOMsg financial(int stan, String pan, String processingCode, Long amountMinor)
      throws Exception {
    ISOMsg request =
        buildIso(
            "0200",
            Map.of(
                3,
                processingCode,
                7,
                "0922120000",
                11,
                pad(stan),
                37,
                "GOCPHO" + pad(stan),
                41,
                "00000042",
                42,
                "GOCPHO000000001"));
    if (amountMinor != null) request.set(4, String.format("%012d", amountMinor));
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

  /** A 0420 (reason 17) for the 0200 sent with {@code originalStan} by {@link #financial}. */
  private static ISOMsg reversal(int stan, int originalStan, String processingCode, long amount)
      throws Exception {
    ISOMsg request =
        buildIso(
            "0420",
            Map.of(
                3,
                processingCode,
                4,
                String.format("%012d", amount),
                7,
                "0922120500",
                11,
                pad(stan),
                32,
                "970499",
                37,
                "GOCPHO" + pad(originalStan),
                39,
                "17",
                41,
                "00000042",
                42,
                "GOCPHO000000001",
                90,
                "0200" + pad(originalStan) + "0922120000" + "00000970499" + "0".repeat(11)));
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

  // ---- POS-G17: refunds credit, reversals mirror, balance inquiries move nothing ----------------

  private static final String SECOND_PAN = "9704360000005540"; // crd_second0006, 200 000 000

  @Test
  @Order(7)
  @DisplayName("POS-G17: a refund credits the customer with a balanced REFUND journal")
  void refundCreditsTheCustomerWithABalancedRefundJournal() throws Exception {
    long accountId = accountIdByAccountNo("ACC-crd_second0006");
    long before = accountBalance(accountId);
    long ledgerBefore = ledgerBalance(accountId);

    ISOMsg response = financial(20, SECOND_PAN, "200000", 25_000L);

    assertThat(response.getString(39)).isEqualTo("00");
    assertThat(response.getString(38)).hasSize(6);
    assertThat(accountBalance(accountId)).isEqualTo(before + 25_000L);
    assertThat(ledgerBalance(accountId)).isEqualTo(ledgerBefore + 25_000L);
    assertThat(postingsOfLatestJournal(accountId, "REFUND"))
        .containsExactlyInAnyOrder("D:SETTLEMENT_SUSPENSE:25000", "C:ACC-crd_second0006:25000");
    assertThat(unbalancedJournalCount()).isZero();
  }

  @Test
  @Order(8)
  @DisplayName("POS-G17: a refund isn't declined for funds or velocity limits")
  void refundIsNotDeclinedForFundsOrVelocity() throws Exception {
    long accountId = accountIdByAccountNo("ACC-crd_lowbal0002");
    long before = accountBalance(accountId);
    long refund = before + 10_000_000L; // more than the balance and than any debit ceiling

    ISOMsg response = financial(21, "9704360000009021", "200000", refund);

    assertThat(response.getString(39)).isEqualTo("00");
    assertThat(accountBalance(accountId)).isEqualTo(before + refund);
  }

  @Test
  @Order(9)
  @DisplayName("POS-G17: reversing a refund debits the customer back")
  void reversingARefundDebitsTheCustomerBack() throws Exception {
    long accountId = accountIdByAccountNo("ACC-crd_second0006");
    long before = accountBalance(accountId);
    long ledgerBefore = ledgerBalance(accountId);
    assertThat(financial(22, SECOND_PAN, "200000", 40_000L).getString(39)).isEqualTo("00");

    ISOMsg ack = reversal(23, 22, "200000", 40_000L);

    assertThat(ack.getMTI()).isEqualTo("0430");
    assertThat(accountBalance(accountId)).isEqualTo(before);
    assertThat(ledgerBalance(accountId)).isEqualTo(ledgerBefore);
    assertThat(postingsOfLatestJournal(accountId, "REVERSAL"))
        .containsExactlyInAnyOrder("C:SETTLEMENT_SUSPENSE:40000", "D:ACC-crd_second0006:40000");
    assertThat(unbalancedJournalCount()).isZero();
  }

  @Test
  @Order(10)
  @DisplayName("POS-G17: reversing a purchase gives the money back")
  void reversingAPurchaseGivesTheMoneyBack() throws Exception {
    long accountId = accountIdByAccountNo("ACC-crd_second0006");
    long before = accountBalance(accountId);
    long ledgerBefore = ledgerBalance(accountId);
    assertThat(purchase(24, SECOND_PAN, 30_000L).getString(39)).isEqualTo("00");
    assertThat(accountBalance(accountId)).isEqualTo(before - 30_000L);

    reversal(25, 24, "000000", 30_000L);
    reversal(26, 24, "000000", 30_000L); // a repeat has no second effect (docs/03 §7.3)

    assertThat(accountBalance(accountId)).isEqualTo(before);
    assertThat(ledgerBalance(accountId)).isEqualTo(ledgerBefore);
    assertThat(unbalancedJournalCount()).isZero();
  }

  @Test
  @Order(11)
  @DisplayName("POS-G17: a balance inquiry posts no journal, holds nothing and answers DE 54")
  void balanceInquiryPostsNothingAndAnswersDe54() throws Exception {
    long accountId = accountIdByAccountNo("ACC-crd_second0006");
    long before = accountBalance(accountId);
    long journalsBefore = journalCount(accountId);

    ISOMsg response = financial(27, SECOND_PAN, "310000", null);

    assertThat(response.getString(39)).isEqualTo("00");
    assertThat(response.getString(54)).isEqualTo("704C" + String.format("%012d", before));
    assertThat(accountBalance(accountId)).isEqualTo(before);
    assertThat(journalCount(accountId)).isEqualTo(journalsBefore);
    assertThat(holdCount(accountId)).isZero();
  }

  // ---- CARDS-G18: authorization reads card status the way the Admin API does ------------------

  @Test
  @Order(12)
  @DisplayName("CARDS-G18: an expired card is declined RC 54")
  void expiredCardIsDeclinedRc54() throws Exception {
    assertThat(purchase(28, "9704360000007765", 10_000L).getString(39)).isEqualTo("54");
  }

  @Test
  @Order(13)
  @DisplayName("CARDS-G18: a PIN_BLOCKED card is declined RC 75 before any PIN check")
  void pinBlockedCardIsDeclinedRc75() throws Exception {
    execute("UPDATE card SET status = 'PIN_BLOCKED' WHERE card_ref = 'crd_limit00005'");
    long accountId = accountIdByAccountNo("ACC-crd_limit00005");
    long before = accountBalance(accountId);

    ISOMsg response = purchase(29, "9704360000001208", 10_000L);

    assertThat(response.getString(39)).isEqualTo("75");
    assertThat(accountBalance(accountId)).isEqualTo(before);
  }

  private static void execute(String sql) throws Exception {
    try (var conn = dataSource.getConnection();
        var stmt = conn.createStatement()) {
      stmt.execute(sql);
    }
  }

  private static long single(String sql, long accountId) throws Exception {
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, accountId);
      try (ResultSet rs = stmt.executeQuery()) {
        rs.next();
        return rs.getLong(1);
      }
    }
  }

  private static long ledgerBalance(long accountId) throws Exception {
    return single("SELECT ledger_balance FROM account WHERE id = ?", accountId);
  }

  private static long journalCount(long accountId) throws Exception {
    return single(
        "SELECT count(DISTINCT journal_id) FROM ledger_posting WHERE account_id = ?", accountId);
  }

  private static long holdCount(long accountId) throws Exception {
    return single(
        "SELECT count(*) FROM auth_hold h JOIN card c ON c.id = h.card_id WHERE c.account_id = ?",
        accountId);
  }

  private static long unbalancedJournalCount() throws Exception {
    try (var conn = dataSource.getConnection();
        var stmt =
            conn.prepareStatement(
                """
                SELECT count(*) FROM (
                  SELECT journal_id FROM ledger_posting GROUP BY journal_id
                  HAVING SUM(CASE WHEN direction = 'D' THEN amount ELSE -amount END) <> 0) x""");
        ResultSet rs = stmt.executeQuery()) {
      rs.next();
      return rs.getLong(1);
    }
  }

  /** "D|C:account-or-GL:amount" for each posting of the account's newest journal of a type. */
  private static java.util.List<String> postingsOfLatestJournal(long accountId, String entryType)
      throws Exception {
    String sql =
        """
        SELECT p.direction, COALESCE(p.gl_code, a.account_no) AS account, p.amount
        FROM ledger_posting p LEFT JOIN account a ON a.id = p.account_id
        WHERE p.journal_id = (
          SELECT max(je.id) FROM journal_entry je JOIN ledger_posting lp ON lp.journal_id = je.id
          WHERE lp.account_id = ? AND je.entry_type = ?)""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, accountId);
      stmt.setString(2, entryType);
      try (ResultSet rs = stmt.executeQuery()) {
        java.util.List<String> postings = new java.util.ArrayList<>();
        while (rs.next()) {
          postings.add(rs.getString(1) + ":" + rs.getString(2) + ":" + rs.getLong(3));
        }
        return postings;
      }
    }
  }
}
