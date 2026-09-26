package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.AccountLockRepository;
import io.mcn.issuer.adapter.persistence.AccountRow;
import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.OriginalTransactionRow;
import io.mcn.issuer.adapter.persistence.ReversalWithoutOriginalRepository;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import java.io.Serializable;
import java.sql.Connection;
import java.sql.SQLException;
import java.time.Duration;
import java.time.LocalDate;
import java.util.Map;
import javax.sql.DataSource;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;
import org.jpos.util.Destroyable;

/**
 * The domain core of a reversal (MCN-402): locates the original transaction by DE 90 (docs/03 §7.3)
 * and takes one of three branches - found and not yet reversed (marks {@code REVERSED}, posts the
 * mirror of the original's journal and moves the account balance by the same amount, all in one
 * transaction: a purchase reversal gives the money back, a refund reversal takes it back), found
 * and already {@code REVERSED} (idempotent no-op, per docs/03 §7.3 "a reversal of an already
 * reversed transaction is acknowledged idempotently with no ledger effect"), or not found ({@code
 * reversal_without_original}, so a later-arriving original is declined RC 94 by {@code
 * Deduplicate}). No branch sets {@code RESPONSE_CODE}; a failure (original still in flight, DB
 * error) aborts, and {@code RespondReversal} then answers 96 so the acquirer repeats.
 */
public class LocateAndReverse implements TransactionParticipant, Configurable, Destroyable {

  private TranLogRepository tranLogRepository;
  private LedgerRepository ledgerRepository;
  private ReversalWithoutOriginalRepository reversalWithoutOriginalRepository;
  private AccountLockRepository accountLockRepository;
  private VelocityCounterRepository velocityCounterRepository;
  private AuditLogRepository auditLogRepository;

  /** Longer than the acquirer's 0200 timeout (docs/03 §9: 30 s) plus a margin. */
  private Duration staleAfter = Duration.ofSeconds(60);

  private DataSource dataSource;
  private HikariDataSource ownedDataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public LocateAndReverse() {}

  public LocateAndReverse(
      TranLogRepository tranLogRepository,
      LedgerRepository ledgerRepository,
      DataSource dataSource) {
    this(
        tranLogRepository,
        ledgerRepository,
        new ReversalWithoutOriginalRepository(dataSource),
        new AccountLockRepository(dataSource),
        dataSource);
  }

  public LocateAndReverse(
      TranLogRepository tranLogRepository,
      LedgerRepository ledgerRepository,
      ReversalWithoutOriginalRepository reversalWithoutOriginalRepository,
      DataSource dataSource) {
    this(
        tranLogRepository,
        ledgerRepository,
        reversalWithoutOriginalRepository,
        new AccountLockRepository(dataSource),
        dataSource);
  }

  public LocateAndReverse(
      TranLogRepository tranLogRepository,
      LedgerRepository ledgerRepository,
      ReversalWithoutOriginalRepository reversalWithoutOriginalRepository,
      AccountLockRepository accountLockRepository,
      DataSource dataSource) {
    this(
        tranLogRepository,
        ledgerRepository,
        reversalWithoutOriginalRepository,
        accountLockRepository,
        new VelocityCounterRepository(dataSource),
        new AuditLogRepository(dataSource),
        dataSource);
  }

  public LocateAndReverse(
      TranLogRepository tranLogRepository,
      LedgerRepository ledgerRepository,
      ReversalWithoutOriginalRepository reversalWithoutOriginalRepository,
      AccountLockRepository accountLockRepository,
      VelocityCounterRepository velocityCounterRepository,
      AuditLogRepository auditLogRepository,
      DataSource dataSource) {
    this.velocityCounterRepository = velocityCounterRepository;
    this.auditLogRepository = auditLogRepository;
    this.tranLogRepository = tranLogRepository;
    this.ledgerRepository = ledgerRepository;
    this.reversalWithoutOriginalRepository = reversalWithoutOriginalRepository;
    this.accountLockRepository = accountLockRepository;
    this.dataSource = dataSource;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.ownedDataSource = TxnDataSource.fromConfig(cfg);
    this.dataSource = ownedDataSource;
    this.tranLogRepository = new TranLogRepository(ownedDataSource);
    this.ledgerRepository = new LedgerRepository();
    this.reversalWithoutOriginalRepository = new ReversalWithoutOriginalRepository(ownedDataSource);
    this.accountLockRepository = new AccountLockRepository(ownedDataSource);
    this.velocityCounterRepository = new VelocityCounterRepository(ownedDataSource);
    this.auditLogRepository = new AuditLogRepository(ownedDataSource);
    this.staleAfter = Duration.ofSeconds(cfg.getLong("stale-received-after-seconds", 60));
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
    String originalMti = ctx.get(TxnContextKeys.ORIGINAL_MTI);
    String originalStan = ctx.get(TxnContextKeys.ORIGINAL_STAN);
    String originalDe7 = ctx.get(TxnContextKeys.ORIGINAL_DE7);
    String originalAcquirer = ctx.get(TxnContextKeys.ORIGINAL_ACQUIRER);
    LocalDate reversalBusinessDate = ctx.get(TxnContextKeys.BUSINESS_DATE);

    var found =
        tranLogRepository.findByReversalKey(
            originalMti, originalStan, originalDe7, originalAcquirer);

    if (found.isEmpty()) {
      reversalWithoutOriginalRepository.insert(
          originalMti,
          originalStan,
          originalDe7,
          originalAcquirer,
          reversalBusinessDate == null ? LocalDate.now() : reversalBusinessDate);
      return PREPARED;
    }

    OriginalTransactionRow original = found.get();
    if ("RECEIVED".equals(original.status())) {
      // Still in flight answers 96 and the acquirer's SAF repeats. A "00" now would let a live
      // 0200 approve and never be reversed - unless the original is stale and moved nothing.
      abandonIfStale(original);
      return PREPARED;
    }
    if (!"APPROVED".equals(original.status())) {
      // Already REVERSED: idempotent repeat, no second journal entry. DECLINED never moved money
      // - most commonly a later original that Deduplicate's RC-94 short-circuit already declined
      // because this same reversal recorded reversal_without_original first; reversing it would
      // fabricate a credit with no matching debit, breaking the double-entry invariant (docs/10
      // §5).
      return PREPARED;
    }

    try (Connection conn = dataSource.getConnection()) {
      conn.setAutoCommit(false);
      if (tranLogRepository.markReversed(conn, original.id(), original.businessDate())) {
        undo(conn, original);
      }
      conn.commit();
    } catch (SQLException e) {
      throw new IllegalStateException("post reversal journal failed", e);
    }
    return PREPARED;
  }

  /**
   * An original still RECEIVED after {@link #staleAfter} with no journal never approved (its
   * process died before Authorize committed, S1): it is abandoned as REVERSED and the reversal
   * acknowledged with no ledger effect. Otherwise it may still be approving, so this fails (96).
   */
  private void abandonIfStale(OriginalTransactionRow original) {
    try (Connection conn = dataSource.getConnection()) {
      if (tranLogRepository.markAbandoned(
          conn, original.id(), original.businessDate(), staleAfter)) {
        return;
      }
    } catch (SQLException e) {
      throw new IllegalStateException("abandon stale original failed", e);
    }
    throw new IllegalStateException("original " + original.id() + " still in flight");
  }

  /**
   * Everything the original did, undone in the caller's transaction: the mirrored journal, the
   * balance it moved (docs/10 §5), and - for a debit - its count against the day's velocity limits
   * (R-3). A reversal always posts, even past the overdraft floor (an advice can't be declined,
   * docs/03 §7.3); that case is recorded for follow-up (R-1).
   */
  private void undo(Connection conn, OriginalTransactionRow original) {
    Map<Long, Long> deltas =
        ledgerRepository.postReversalOf(conn, original.id(), original.businessDate());
    for (var entry : deltas.entrySet()) {
      AccountRow after = accountLockRepository.adjust(conn, entry.getKey(), entry.getValue());
      boolean debitedTheCustomer = entry.getValue() < 0; // only a debit can cross the floor
      if (debitedTheCustomer && after.availableBalance() < -after.overdraftLimit()) {
        recordNegativeBalance(conn, original, entry.getValue(), after);
      }
    }
    boolean originalDebitedTheCustomer =
        deltas.values().stream().mapToLong(Long::longValue).sum() > 0;
    if (originalDebitedTheCustomer) {
      velocityCounterRepository.decrementDaily(
          conn,
          original.cardId(),
          VelocityCounterRepository.DEBIT_TRAN_TYPE,
          original.businessDate(),
          original.amount());
    }
  }

  private void recordNegativeBalance(
      Connection conn, OriginalTransactionRow original, long delta, AccountRow after) {
    auditLogRepository.record(
        conn,
        "issuer",
        "NEGATIVE_BALANCE_AFTER_REVERSAL",
        "account",
        String.valueOf(after.id()),
        "{\"availableBalance\":" + (after.availableBalance() - delta) + "}",
        "{\"availableBalance\":"
            + after.availableBalance()
            + ",\"overdraftLimit\":"
            + after.overdraftLimit()
            + ",\"reversedTranId\":"
            + original.id()
            + ",\"businessDate\":\""
            + original.businessDate()
            + "\"}");
  }
}
