package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;

import io.mcn.issuer.adapter.persistence.AccountLockRepository;
import io.mcn.issuer.adapter.persistence.AccountRepository;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.ReversalWithoutOriginalRepository;
import io.mcn.issuer.adapter.persistence.TestDataSources;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.TranLogRow;
import java.nio.charset.StandardCharsets;
import java.sql.ResultSet;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.List;
import java.util.Random;
import javax.sql.DataSource;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * For every shuffled ordering of {original, duplicate-of-original, reversal, repeat-of-reversal}
 * (200 seeds), drives the real participants directly against a fresh account/card and asserts the
 * account's ledger postings stay balanced (docs/plans/MCN-402.md Ruling 2 - a manual seeded random
 * loop, no new property-testing dependency). On failure the assertion message names the seed and
 * order for reproduction.
 */
@Testcontainers
class ReversalInterleavingPropertyTest {

  private enum Event {
    ORIGINAL,
    DUPLICATE_ORIGINAL,
    REVERSAL,
    REPEAT_REVERSAL
  }

  private static final long AMOUNT = 10_000L;
  private static final String ACQUIRER_ID = "970499";
  private static final String CURRENCY = "704";

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test")
          .withCommand("postgres", "-c", "synchronous_commit=off");

  @Test
  void anyInterleavingOfOriginalDuplicateReversalRepeat_leavesLedgerBalanced_MCN_402_AC4() {
    DataSource ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    var cards = new CardRepository(ds);
    var tranLog = new TranLogRepository(ds);
    var ledger = new LedgerRepository();
    var rwo = new ReversalWithoutOriginalRepository(ds);
    var deduplicate = new Deduplicate(tranLog, rwo);
    var authorize =
        new io.mcn.issuer.adapter.txn.Authorize(
            new AccountLockRepository(ds), ledger, new AuthCodeGenerator(), ds);
    var locateAndReverse = new LocateAndReverse(tranLog, ledger, rwo, cards, ds);
    LocalDate businessDate = LocalDate.now();

    for (int seed = 0; seed < 200; seed++) {
      List<Event> order = shuffled(seed);

      String stan = String.format("%06d", seed);
      long accountId = accounts.insert("ACC-RIT-" + seed, CURRENCY, AMOUNT);
      long cardId =
          cards.insert(
              accountId,
              ("enc" + seed).getBytes(StandardCharsets.UTF_8),
              ("hash" + seed).getBytes(StandardCharsets.UTF_8),
              "970436",
              "0001",
              "3012",
              "ACTIVE");

      for (Event event : order) {
        switch (event) {
          case ORIGINAL ->
              runOriginal(tranLog, deduplicate, authorize, businessDate, accountId, cardId, stan);
          case DUPLICATE_ORIGINAL -> runDuplicateOriginal(deduplicate, businessDate, stan);
          case REVERSAL, REPEAT_REVERSAL -> runReversal(locateAndReverse, businessDate, stan);
        }
      }

      long netPosted = sumPostingsForAccount(ds, accountId);
      assertThat(netPosted).as("seed=%d order=%s", seed, order).isZero();
    }
  }

  /**
   * Mirrors the real chain's relevant slice: {@code Deduplicate} (whose RC-94 short-circuit is the
   * mechanism that keeps the ledger balanced when a reversal-without-original beat the original
   * here) runs before {@code Authorize}, whose own guard skips debiting once RC 94 is set.
   */
  private static void runOriginal(
      TranLogRepository tranLog,
      Deduplicate deduplicate,
      io.mcn.issuer.adapter.txn.Authorize authorize,
      LocalDate businessDate,
      long accountId,
      long cardId,
      String stan) {
    ISOMsg request = originalRequest(stan);
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.ACQUIRER_ID, ACQUIRER_ID);
    ctx.put(TxnContextKeys.BUSINESS_DATE, businessDate);
    deduplicate.prepare(0, ctx);

    long tranId = tranLog.insert(receivedTranLogRow(businessDate, stan, cardId));
    ctx.put(TxnContextKeys.ACCOUNT_ID, accountId);
    ctx.put(TxnContextKeys.AMOUNT, AMOUNT);
    ctx.put(TxnContextKeys.TRAN_ID, tranId);
    authorize.prepare(0, ctx);

    // Mirrors LogAndOutbox finalizing the row's outcome - by the time any reversal could
    // physically be processed in production, tran_log.status is already final, never RECEIVED.
    String responseCode = ctx.get(TxnContextKeys.RESPONSE_CODE);
    tranLog.updateOutcome(
        tranId,
        businessDate,
        "00".equals(responseCode) ? "APPROVED" : "DECLINED",
        responseCode,
        ctx.get(TxnContextKeys.AUTH_CODE),
        null);
  }

  private static void runDuplicateOriginal(
      Deduplicate deduplicate, LocalDate businessDate, String stan) {
    ISOMsg msg = originalRequest(stan);
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, msg);
    ctx.put(TxnContextKeys.ACQUIRER_ID, ACQUIRER_ID);
    ctx.put(TxnContextKeys.BUSINESS_DATE, businessDate);
    deduplicate.prepare(0, ctx);
    // A duplicate never re-authorizes (production's Authorize.prepare guards on RESPONSE_CODE
    // already set); this call only proves Deduplicate itself doesn't mutate the ledger.
  }

  private static void runReversal(
      LocateAndReverse locateAndReverse, LocalDate businessDate, String stan) {
    Context ctx = new Context();
    ctx.put(TxnContextKeys.ORIGINAL_MTI, "0200");
    ctx.put(TxnContextKeys.ORIGINAL_STAN, stan);
    ctx.put(TxnContextKeys.ORIGINAL_DE7, "0922140000");
    ctx.put(TxnContextKeys.ORIGINAL_ACQUIRER, ACQUIRER_ID);
    ctx.put(TxnContextKeys.BUSINESS_DATE, businessDate);
    locateAndReverse.prepare(0, ctx);
  }

  private static ISOMsg originalRequest(String stan) {
    ISOMsg msg = new ISOMsg("0200");
    msg.set(7, "0922140000");
    msg.set(11, stan);
    msg.set(41, "GOCPHO00");
    return msg;
  }

  private static long sumPostingsForAccount(DataSource ds, long accountId) {
    String sql =
        "SELECT COALESCE(SUM(CASE WHEN direction = 'D' THEN amount ELSE 0 END), 0)"
            + " - COALESCE(SUM(CASE WHEN direction = 'C' THEN amount ELSE 0 END), 0)"
            + " AS net FROM ledger_posting WHERE account_id = ?";
    try (var conn = ds.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, accountId);
      try (ResultSet rs = stmt.executeQuery()) {
        rs.next();
        return rs.getLong("net");
      }
    } catch (java.sql.SQLException e) {
      throw new IllegalStateException("sum ledger postings failed", e);
    }
  }

  private static List<Event> shuffled(int seed) {
    List<Event> order = new ArrayList<>(List.of(Event.values()));
    java.util.Collections.shuffle(order, new Random(seed));
    return order;
  }

  private static TranLogRow receivedTranLogRow(LocalDate businessDate, String stan, long cardId) {
    return new TranLogRow(
        businessDate,
        "0200",
        "PURCHASE",
        "000000",
        ACQUIRER_ID,
        "GOCPHO00",
        "GOCPHO000000001",
        stan,
        "0922140000",
        "RRN" + stan.substring(0, Math.min(9, stan.length())),
        AMOUNT,
        CURRENCY,
        cardId,
        "RECEIVED",
        null,
        null,
        null);
  }
}
