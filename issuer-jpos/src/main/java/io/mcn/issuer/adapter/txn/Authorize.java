package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.AccountLockRepository;
import io.mcn.issuer.adapter.persistence.AccountRow;
import io.mcn.issuer.adapter.persistence.AccountSummary;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import java.io.Serializable;
import java.sql.Connection;
import java.sql.SQLException;
import java.time.LocalDate;
import javax.sql.DataSource;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;
import org.jpos.util.Destroyable;

/**
 * Real approval (MCN-302b, replacing MCN-302a's RC-96 placeholder), by what the request does to the
 * customer ({@link io.mcn.issuer.domain.TransactionType}): a purchase row-locks the account and
 * debits it with a balanced journal, or declines RC 51; a refund credits it with a balanced REFUND
 * journal (never declined for funds); a balance inquiry answers DE 54 and posts nothing. Every
 * approval carries RC 00 + a DE 38 auth code. Never touches the account when an earlier participant
 * already set {@code RESPONSE_CODE} (a decline, or a replayed duplicate).
 *
 * <p><b>Task 0 finding:</b> jPOS 3.0.1's {@code TransactionManager} hands every participant in a
 * transaction the same {@code Context}, but never a shared JDBC {@code Connection} - confirmed both
 * from {@code TransactionManager}'s source (each participant is {@code Configurable} and wires up
 * its own resources in {@code setConfiguration}; the manager only ever calls {@code prepare}/{@code
 * commit}/{@code abort} with the {@code Context}) and from every participant already merged in this
 * codebase (CheckCard, CheckLimits, Deduplicate, LogAndOutbox each build their own {@link
 * HikariDataSource} from Q2 config via {@link TxnDataSource}). A row lock taken on one
 * participant's connection would therefore be silently released the moment that connection is
 * returned to its pool - long before a later participant's {@code commit()} runs. So the row lock,
 * the debit and the journal insert all happen here, in {@link #prepare}, inside one explicit JDBC
 * transaction on one connection this class opens and closes itself; nothing is deferred to {@code
 * commit()}/{@code abort()} (this participant does not implement {@code AbortParticipant} - there
 * is nothing left to compensate for once {@code prepare()} has already committed or rolled back its
 * own transaction).
 *
 * <p>Needs {@code LogAndOutbox} to have already run (it precedes this participant in {@code
 * 40_txn_manager.xml}) so {@code TxnContextKeys.TRAN_ID} names an already-committed {@code
 * tran_log} row - {@code journal_entry.tran_id} has a real, non-deferred FK to it.
 */
public class Authorize implements TransactionParticipant, Configurable, Destroyable {

  private static final String CURRENCY = "704";

  private AccountLockRepository lockRepository;
  private LedgerRepository ledgerRepository;
  private VelocityCounterRepository velocityCounterRepository;
  private TranLogRepository tranLogRepository;
  private final AuthCodeGenerator authCodeGenerator;
  private DataSource dataSource;
  private HikariDataSource ownedDataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public Authorize() {
    this(null, null, null, new AuthCodeGenerator(), null);
  }

  public Authorize(
      AccountLockRepository lockRepository,
      LedgerRepository ledgerRepository,
      VelocityCounterRepository velocityCounterRepository,
      AuthCodeGenerator authCodeGenerator) {
    this(lockRepository, ledgerRepository, velocityCounterRepository, authCodeGenerator, null);
  }

  public Authorize(
      AccountLockRepository lockRepository,
      LedgerRepository ledgerRepository,
      VelocityCounterRepository velocityCounterRepository,
      AuthCodeGenerator authCodeGenerator,
      DataSource dataSource) {
    this(
        lockRepository,
        ledgerRepository,
        velocityCounterRepository,
        authCodeGenerator,
        dataSource == null ? null : new TranLogRepository(dataSource),
        dataSource);
  }

  public Authorize(
      AccountLockRepository lockRepository,
      LedgerRepository ledgerRepository,
      VelocityCounterRepository velocityCounterRepository,
      AuthCodeGenerator authCodeGenerator,
      TranLogRepository tranLogRepository,
      DataSource dataSource) {
    this.tranLogRepository = tranLogRepository;
    this.lockRepository = lockRepository;
    this.ledgerRepository = ledgerRepository;
    this.velocityCounterRepository = velocityCounterRepository;
    this.authCodeGenerator = authCodeGenerator;
    this.dataSource = dataSource;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.ownedDataSource = TxnDataSource.fromConfig(cfg);
    this.dataSource = ownedDataSource;
    this.lockRepository = new AccountLockRepository(this.dataSource);
    this.ledgerRepository = new LedgerRepository();
    this.velocityCounterRepository = new VelocityCounterRepository(this.dataSource);
    this.tranLogRepository = new TranLogRepository(this.dataSource);
  }

  @Override
  public void destroy() {
    if (ownedDataSource != null) {
      ownedDataSource.close();
    }
  }

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    if (ctx.<String>get(TxnContextKeys.RESPONSE_CODE) != null) {
      return PREPARED;
    }
    try (Connection conn = dataSource.getConnection()) {
      conn.setAutoCommit(false);
      try {
        return approveInOneTransaction(conn, ctx);
      } catch (ReversedBeforeApproval e) {
        conn.rollback();
        // docs/03 §7.3 and §8: an original processed after its reversal is declined RC 94
        ctx.put(TxnContextKeys.RESPONSE_CODE, "94");
        ctx.put(TxnContextKeys.DECLINE_REASON, "reversed before approval");
        return PREPARED;
      } catch (RuntimeException e) {
        conn.rollback();
        throw e;
      }
    } catch (SQLException e) {
      throw new IllegalStateException("authorize failed", e);
    }
  }

  /**
   * Moves the money and records the APPROVED outcome on the {@code tran_log} row in one transaction
   * (S1, root CLAUDE.md §6.5): a crash can't leave money moved behind a RECEIVED row that every
   * reversal would bounce off. {@code LogAndOutbox} later rewrites the same outcome and owns every
   * decline's, where nothing moved.
   */
  private int approveInOneTransaction(Connection conn, Context ctx) throws SQLException {
    boolean approved =
        switch (TxnTypes.of(ctx).customerEffect()) {
          case DEBIT -> debit(conn, ctx);
          case CREDIT -> credit(conn, ctx);
          case NONE -> answerBalance(conn, ctx);
        };
    if (!approved) {
      conn.rollback();
      return PREPARED;
    }
    String authCode = authCodeGenerator.generate();
    boolean recorded =
        tranLogRepository.updateOutcome(
            conn,
            tranId(ctx),
            businessDate(ctx),
            "APPROVED",
            "00",
            authCode,
            null,
            ctx.<Long>get(TxnContextKeys.BALANCE));
    if (!recorded) {
      throw new ReversedBeforeApproval(); // a reversal abandoned the row while we held the money
    }
    conn.commit();
    ctx.put(TxnContextKeys.RESPONSE_CODE, "00");
    ctx.put(TxnContextKeys.AUTH_CODE, authCode);
    return PREPARED;
  }

  /**
   * Purchase or cash: RC 51 unless the balance stays at or above the overdraft floor ({@code
   * -overdraft_limit}), checked here under the row lock because the account table no longer
   * enforces it (reversals must be able to overdraw); then debit, journal and velocity.
   */
  private boolean debit(Connection conn, Context ctx) {
    long accountId = ctx.get(TxnContextKeys.ACCOUNT_ID);
    long amount = ctx.get(TxnContextKeys.AMOUNT);
    AccountRow account = lockRepository.lockAndGet(conn, accountId);
    if (account.availableBalance() - amount < -account.overdraftLimit()) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "51");
      ctx.put(TxnContextKeys.DECLINE_REASON, "insufficient funds");
      return false;
    }
    lockRepository.adjust(conn, accountId, -amount);
    LocalDate businessDate = businessDate(ctx);
    ledgerRepository.postPurchase(conn, tranId(ctx), businessDate, accountId, amount, CURRENCY);
    Long cardId = ctx.get(TxnContextKeys.CARD_ID);
    if (cardId != null) {
      velocityCounterRepository.incrementDaily(
          conn, cardId, VelocityCounterRepository.DEBIT_TRAN_TYPE, businessDate, amount);
    }
    return true;
  }

  /**
   * Refund: the money goes to the customer, so there is no funds check and it doesn't count towards
   * the debit velocity counters (POS-G17).
   */
  private boolean credit(Connection conn, Context ctx) {
    long accountId = ctx.get(TxnContextKeys.ACCOUNT_ID);
    long amount = ctx.get(TxnContextKeys.AMOUNT);
    lockRepository.adjust(conn, accountId, amount);
    ledgerRepository.postRefund(conn, tranId(ctx), businessDate(ctx), accountId, amount, CURRENCY);
    return true;
  }

  /** Balance inquiry: answers the available balance in DE 54; posts and holds nothing. */
  private boolean answerBalance(Connection conn, Context ctx) {
    AccountSummary account = lockRepository.read(conn, ctx.get(TxnContextKeys.ACCOUNT_ID));
    ctx.put(TxnContextKeys.BALANCE, account.availableBalance());
    ctx.put(TxnContextKeys.BALANCE_CURRENCY, account.currency());
    return true;
  }

  /**
   * The row is no longer RECEIVED: {@code LocateAndReverse} abandoned it as stale while this
   * approval was in progress, so the approval must roll back.
   */
  private static final class ReversedBeforeApproval extends RuntimeException {
    ReversedBeforeApproval() {
      super("tran_log row was reversed before its approval committed", null, false, false);
    }
  }

  /** The row {@code LogAndOutbox} inserted; an approval with none has nothing to post against. */
  private static long tranId(Context ctx) {
    Long tranId = ctx.get(TxnContextKeys.TRAN_ID);
    if (tranId == null) {
      throw new IllegalStateException("no tran_log row (TRAN_ID) to record the approval on");
    }
    return tranId;
  }

  private static LocalDate businessDate(Context ctx) {
    LocalDate businessDate = ctx.get(TxnContextKeys.BUSINESS_DATE);
    return businessDate == null ? LocalDate.now() : businessDate;
  }
}
