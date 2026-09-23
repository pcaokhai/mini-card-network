package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.OriginalTransactionRow;
import io.mcn.issuer.adapter.persistence.ReversalWithoutOriginalRepository;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
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
 * The domain core of a reversal (MCN-402): locates the original transaction by DE 90 (docs/03 §7.3)
 * and takes one of three branches - found and not yet reversed (posts the reversing journal, marks
 * {@code REVERSED}), found and already {@code REVERSED} (idempotent no-op, per docs/03 §7.3 "a
 * reversal of an already reversed transaction is acknowledged idempotently with no ledger effect"),
 * or not found ({@code reversal_without_original}, so a later-arriving original is declined RC 94
 * by {@code Deduplicate}). No branch ever sets {@code RESPONSE_CODE} - advices are always ACKed
 * with 0430 by {@code RespondReversal}.
 */
public class LocateAndReverse implements TransactionParticipant, Configurable, Destroyable {

  private TranLogRepository tranLogRepository;
  private LedgerRepository ledgerRepository;
  private ReversalWithoutOriginalRepository reversalWithoutOriginalRepository;
  private CardRepository cardRepository;
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
        new CardRepository(dataSource),
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
        new CardRepository(dataSource),
        dataSource);
  }

  public LocateAndReverse(
      TranLogRepository tranLogRepository,
      LedgerRepository ledgerRepository,
      ReversalWithoutOriginalRepository reversalWithoutOriginalRepository,
      CardRepository cardRepository,
      DataSource dataSource) {
    this.tranLogRepository = tranLogRepository;
    this.ledgerRepository = ledgerRepository;
    this.reversalWithoutOriginalRepository = reversalWithoutOriginalRepository;
    this.cardRepository = cardRepository;
    this.dataSource = dataSource;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.ownedDataSource = TxnDataSource.fromConfig(cfg);
    this.dataSource = ownedDataSource;
    this.tranLogRepository = new TranLogRepository(ownedDataSource);
    this.ledgerRepository = new LedgerRepository();
    this.reversalWithoutOriginalRepository = new ReversalWithoutOriginalRepository(ownedDataSource);
    this.cardRepository = new CardRepository(ownedDataSource);
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
    if (!"APPROVED".equals(original.status())) {
      // Already REVERSED: idempotent repeat, no second journal entry. Anything else (DECLINED,
      // RECEIVED) never moved money in the first place - most commonly a later original that
      // Deduplicate's RC-94 short-circuit already declined because this same reversal recorded
      // reversal_without_original first; reversing it would fabricate a credit with no matching
      // debit, breaking the double-entry invariant (docs/10 §5).
      return PREPARED;
    }

    long accountId =
        cardRepository
            .findById(original.cardId())
            .orElseThrow(
                () -> new IllegalStateException("card " + original.cardId() + " not found"))
            .accountId();

    try (Connection conn = dataSource.getConnection()) {
      conn.setAutoCommit(false);
      ledgerRepository.postReversal(
          conn,
          original.id(),
          original.businessDate(),
          accountId,
          original.amount(),
          original.currency());
      conn.commit();
    } catch (SQLException e) {
      throw new IllegalStateException("post reversal journal failed", e);
    }
    tranLogRepository.markReversed(original.id(), original.businessDate());
    return PREPARED;
  }
}
