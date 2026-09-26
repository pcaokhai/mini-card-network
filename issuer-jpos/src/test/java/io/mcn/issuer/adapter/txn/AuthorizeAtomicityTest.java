package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.mcn.issuer.adapter.persistence.AccountLockRepository;
import io.mcn.issuer.adapter.persistence.AccountRepository;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.ReversalWithoutOriginalRepository;
import io.mcn.issuer.adapter.persistence.TestDataSources;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.TranLogRow;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.time.LocalDate;
import javax.sql.DataSource;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * S1 (#119 review): the money an approval moves and the {@code tran_log} row that says so commit
 * together (root CLAUDE.md §6.5), so no crash can leave money moved behind a RECEIVED row that
 * every reversal would bounce off (96, "still in flight") until the SAF gives up.
 */
@Testcontainers
class AuthorizeAtomicityTest {
  private static final long OPENING = 100_000L;
  private static final long AMOUNT = 10_000L;
  private static final LocalDate TODAY = LocalDate.now();

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  private DataSource ds;
  private TranLogRepository tranLog;
  private long accountId;
  private long cardId;
  private long tranId;
  private String stan;

  @BeforeEach
  void seed() {
    ds = TestDataSources.migrated(postgres);
    tranLog = new TranLogRepository(ds);
    stan = String.format("%06d", System.nanoTime() % 1_000_000);
    accountId = new AccountRepository(ds).insert("ACC-AT-" + stan, "704", OPENING);
    cardId =
        new CardRepository(ds)
            .insert(
                accountId,
                ("enc" + stan).getBytes(StandardCharsets.UTF_8),
                ("hash" + stan).getBytes(StandardCharsets.UTF_8),
                "970436",
                "0001",
                "3012",
                "ACTIVE");
    tranId =
        tranLog.insert(
            new TranLogRow(
                TODAY,
                "0200",
                "PURCHASE",
                "000000",
                "970499",
                "GOCPHO00",
                "GOCPHO000000001",
                stan,
                "0922140000",
                "RRN000" + stan,
                AMOUNT,
                "704",
                cardId,
                "RECEIVED",
                null,
                null,
                null));
  }

  private Authorize authorize(TranLogRepository tranLogRepository) {
    return new Authorize(
        new AccountLockRepository(ds),
        new LedgerRepository(),
        new VelocityCounterRepository(ds),
        new AuthCodeGenerator(),
        tranLogRepository,
        ds);
  }

  private Context purchaseContext() {
    Context ctx = new Context();
    ctx.put(TxnContextKeys.ACCOUNT_ID, accountId);
    ctx.put(TxnContextKeys.CARD_ID, cardId);
    ctx.put(TxnContextKeys.AMOUNT, AMOUNT);
    ctx.put(TxnContextKeys.TRAN_ID, tranId);
    ctx.put(TxnContextKeys.BUSINESS_DATE, TODAY);
    return ctx;
  }

  @Test
  @DisplayName("S1: a failure after the journal insert rolls back the debit, journal and velocity")
  void should_rollBackEverything_when_theOutcomeWriteFailsAfterTheJournal() throws Exception {
    var failingTranLog =
        new TranLogRepository(ds) {
          @Override
          public boolean updateOutcome(
              Connection conn,
              long id,
              LocalDate businessDate,
              String status,
              String responseCode,
              String authCode,
              String declineReason,
              Long balance) {
            throw new IllegalStateException("injected: crash after the journal insert");
          }
        };

    assertThatThrownBy(() -> authorize(failingTranLog).prepare(1L, purchaseContext()))
        .isInstanceOf(IllegalStateException.class);

    assertThat(scalar("SELECT available_balance FROM account WHERE id = " + accountId))
        .isEqualTo(OPENING);
    assertThat(scalar("SELECT ledger_balance FROM account WHERE id = " + accountId))
        .isEqualTo(OPENING);
    assertThat(scalar("SELECT count(*) FROM journal_entry WHERE tran_id = " + tranId)).isZero();
    assertThat(scalar("SELECT count(*) FROM velocity_counter WHERE card_id = " + cardId)).isZero();
    assertThat(status()).isEqualTo("RECEIVED");
  }

  @Test
  @DisplayName(
      "S1: once Authorize commits, the row already reads APPROVED - a crash can't strand it")
  void should_commitTheApprovedOutcomeWithTheMoney_when_authorizeApproves() throws Exception {
    Context ctx = purchaseContext();

    authorize(tranLog).prepare(1L, ctx); // and then the process dies: LogAndOutbox never finishes

    assertThat(status()).isEqualTo("APPROVED");
    assertThat(text("SELECT response_code FROM tran_log WHERE id = " + tranId)).isEqualTo("00");
    assertThat(text("SELECT auth_code FROM tran_log WHERE id = " + tranId))
        .isEqualTo(ctx.<String>get(TxnContextKeys.AUTH_CODE));

    Context reversal = new Context();
    reversal.put(TxnContextKeys.ORIGINAL_MTI, "0200");
    reversal.put(TxnContextKeys.ORIGINAL_STAN, stan);
    reversal.put(TxnContextKeys.ORIGINAL_DE7, "0922140000");
    reversal.put(TxnContextKeys.ORIGINAL_ACQUIRER, "970499");
    reversal.put(TxnContextKeys.BUSINESS_DATE, TODAY);
    new LocateAndReverse(
            tranLog,
            new LedgerRepository(),
            new ReversalWithoutOriginalRepository(ds),
            new AccountLockRepository(ds),
            ds)
        .prepare(2L, reversal);

    assertThat(status()).isEqualTo("REVERSED");
    assertThat(scalar("SELECT available_balance FROM account WHERE id = " + accountId))
        .isEqualTo(OPENING);
  }

  @Test
  @DisplayName("N5: an approval without a tran_log row to post against fails loudly")
  void should_fail_when_thereIsNoTranLogRow() {
    Context ctx = purchaseContext();
    ctx.remove(TxnContextKeys.TRAN_ID);

    assertThatThrownBy(() -> authorize(tranLog).prepare(1L, ctx))
        .isInstanceOf(IllegalStateException.class);
    assertThat(scalar("SELECT available_balance FROM account WHERE id = " + accountId))
        .isEqualTo(OPENING);
  }

  private Context reversalContext() {
    Context reversal = new Context();
    reversal.put(TxnContextKeys.ORIGINAL_MTI, "0200");
    reversal.put(TxnContextKeys.ORIGINAL_STAN, stan);
    reversal.put(TxnContextKeys.ORIGINAL_DE7, "0922140000");
    reversal.put(TxnContextKeys.ORIGINAL_ACQUIRER, "970499");
    reversal.put(TxnContextKeys.BUSINESS_DATE, TODAY);
    return reversal;
  }

  private LocateAndReverse locateAndReverse() {
    return new LocateAndReverse(
        tranLog,
        new LedgerRepository(),
        new ReversalWithoutOriginalRepository(ds),
        new AccountLockRepository(ds),
        ds);
  }

  private void ageTheRow() {
    execute("UPDATE tran_log SET created_at = now() - interval '2 minutes' WHERE id = " + tranId);
  }

  @Test
  @DisplayName(
      "Stale RECEIVED: an original older than the timeout with no journal is abandoned -"
          + " REVERSED, 0430 00, nothing posted")
  void should_abandonTheOriginal_when_itIsStaleAndMovedNoMoney() {
    ageTheRow();

    int result = locateAndReverse().prepare(2L, reversalContext());

    assertThat(result & org.jpos.transaction.TransactionConstants.PREPARED)
        .isEqualTo(org.jpos.transaction.TransactionConstants.PREPARED);
    assertThat(status()).isEqualTo("REVERSED");
    assertThat(scalar("SELECT count(*) FROM journal_entry WHERE tran_id = " + tranId)).isZero();
    assertThat(scalar("SELECT available_balance FROM account WHERE id = " + accountId))
        .isEqualTo(OPENING);
  }

  @Test
  @DisplayName("Stale RECEIVED: a fresh RECEIVED original is still in flight - 96, SAF repeats")
  void should_stillFail_when_theReceivedOriginalIsFresh() {
    assertThatThrownBy(() -> locateAndReverse().prepare(2L, reversalContext()))
        .isInstanceOf(IllegalStateException.class);
    assertThat(status()).isEqualTo("RECEIVED");
  }

  @Test
  @DisplayName(
      "Stale RECEIVED: when the reversal wins, a late Authorize rolls back and declines RC 94,"
          + " and LogAndOutbox keeps the row REVERSED")
  void should_rollBackALateApproval_when_theReversalAbandonedTheRowFirst() {
    ageTheRow();
    locateAndReverse().prepare(2L, reversalContext());
    Context late = purchaseContext();

    authorize(tranLog).prepare(1L, late);
    new LogAndOutbox(tranLog).commit(1L, late);

    assertThat(late.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("94");
    assertThat(status()).isEqualTo("REVERSED");
    assertThat(scalar("SELECT available_balance FROM account WHERE id = " + accountId))
        .isEqualTo(OPENING);
    assertThat(scalar("SELECT count(*) FROM journal_entry WHERE tran_id = " + tranId)).isZero();
    assertThat(scalar("SELECT count(*) FROM velocity_counter WHERE card_id = " + cardId)).isZero();
  }

  @Test
  @DisplayName("Stale RECEIVED: LogAndOutbox never overwrites a row that already has its outcome")
  void should_notOverwriteAnApprovedThenReversedRow_when_logAndOutboxFinishesLate() {
    Context ctx = purchaseContext();
    authorize(tranLog).prepare(1L, ctx);
    locateAndReverse().prepare(2L, reversalContext()); // a fast reversal before finish() runs

    new LogAndOutbox(tranLog).commit(1L, ctx);

    assertThat(status()).isEqualTo("REVERSED");
  }

  private void execute(String sql) {
    try (var conn = ds.getConnection()) {
      conn.createStatement().execute(sql);
    } catch (java.sql.SQLException e) {
      throw new IllegalStateException(e);
    }
  }

  private String status() {
    return text("SELECT status FROM tran_log WHERE id = " + tranId);
  }

  private long scalar(String sql) {
    try (var conn = ds.getConnection();
        var rs = conn.createStatement().executeQuery(sql)) {
      rs.next();
      return rs.getLong(1);
    } catch (java.sql.SQLException e) {
      throw new IllegalStateException(e);
    }
  }

  private String text(String sql) {
    try (var conn = ds.getConnection();
        var rs = conn.createStatement().executeQuery(sql)) {
      rs.next();
      return rs.getString(1) == null ? null : rs.getString(1).trim();
    } catch (java.sql.SQLException e) {
      throw new IllegalStateException(e);
    }
  }
}
