package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.TranLogRow;
import java.io.Serializable;
import java.time.LocalDate;
import java.util.Map;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.AbortParticipant;
import org.jpos.transaction.Context;
import org.jpos.util.Destroyable;

/**
 * Writes the {@code tran_log} row for every attempt (docs/plans/MCN-302a.md Global Constraints) -
 * approved, declined, or the RC-96 placeholder. Implements {@link AbortParticipant} and overrides
 * {@code abort()} (not just {@code commit()}): a decline earlier in the chain makes the *whole*
 * jPOS transaction abort, and TransactionManager calls {@code abort()}, not {@code commit()}, on
 * every member when that happens - only {@code AbortParticipant}s stay "members" at all once a
 * transaction is already known to be aborting (see jPOS's {@code prepareForAbort}). A duplicate hit
 * is not re-logged (the row already exists; re-inserting would violate {@code uq_tran_dedupe}).
 *
 * <p>MCN-302b: {@code journal_entry.tran_id} has a real (non-deferred) FK to {@code tran_log(id,
 * business_date)}, so {@code Authorize} needs an already-committed {@code tran_log} row to post its
 * ledger entry against. This participant is placed right after {@code CheckCard} in {@code
 * 40_txn_manager.xml} (before {@code VerifySecurity}/{@code CheckLimits}/{@code Authorize}) and
 * {@link #prepare} inserts a {@code RECEIVED} row up front (own connection, autocommit - each
 * participant here owns its own {@link com.zaxxer.hikari.HikariDataSource}, confirmed against jPOS
 * 3.0.1's {@code TransactionManager}, which hands every participant the same {@code Context} but
 * never a shared JDBC {@code Connection}), stashing {@code TRAN_ID} in the {@code Context} so any
 * later participant that reaches {@code Authorize} can reference a row that already exists. {@code
 * commit()}/{@code abort()} then just {@code UPDATE} that row with the final outcome. If the chain
 * never reaches this participant's {@code prepare()} (an earlier participant aborted first), {@code
 * TRAN_ID} stays unset and {@code abort()} falls back to the old single-insert behavior.
 */
public class LogAndOutbox implements AbortParticipant, Configurable, Destroyable {

  private static final Map<String, String> TRAN_TYPE_BY_PROCESSING_CODE =
      Map.of(
          "000000", "PURCHASE",
          "010000", "CASH",
          "200000", "REFUND",
          "310000", "BALANCE");

  private TranLogRepository tranLogRepository;
  private HikariDataSource dataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public LogAndOutbox() {}

  public LogAndOutbox(TranLogRepository tranLogRepository) {
    this.tranLogRepository = tranLogRepository;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.dataSource = TxnDataSource.fromConfig(cfg);
    this.tranLogRepository = new TranLogRepository(dataSource);
  }

  @Override
  public void destroy() {
    if (dataSource != null) {
      dataSource.close();
    }
  }

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    if (Boolean.TRUE.equals(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE))) {
      return PREPARED;
    }
    TranLogRow row = buildRow(ctx, "RECEIVED", null, null, null);
    long tranId = tranLogRepository.insert(row);
    ctx.put(TxnContextKeys.TRAN_ID, tranId);
    return PREPARED;
  }

  @Override
  public void commit(long id, Serializable context) {
    finish(context);
  }

  @Override
  public void abort(long id, Serializable context) {
    finish(context);
  }

  private void finish(Serializable context) {
    Context ctx = (Context) context;
    if (Boolean.TRUE.equals(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE))) {
      return;
    }

    String responseCode = ctx.get(TxnContextKeys.RESPONSE_CODE);
    String declineReason = ctx.get(TxnContextKeys.DECLINE_REASON);
    String authCode = ctx.get(TxnContextKeys.AUTH_CODE);
    String status = "00".equals(responseCode) ? "APPROVED" : "DECLINED";
    Long tranId = ctx.get(TxnContextKeys.TRAN_ID);

    if (tranId != null) {
      LocalDate businessDate = ctx.get(TxnContextKeys.BUSINESS_DATE);
      tranLogRepository.updateOutcome(
          tranId,
          businessDate == null ? LocalDate.now() : businessDate,
          status,
          responseCode,
          authCode,
          declineReason);
    } else {
      // ponytail: prepare() never ran (an earlier participant aborted before this one's turn) -
      // one-shot insert with the outcome already known, same as MCN-302a's original behavior.
      tranLogRepository.insert(buildRow(ctx, status, responseCode, authCode, declineReason));
    }
  }

  private TranLogRow buildRow(
      Context ctx, String status, String responseCode, String authCode, String declineReason) {
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);
    Long amount = ctx.get(TxnContextKeys.AMOUNT);
    Long cardId = ctx.get(TxnContextKeys.CARD_ID);
    String processingCode = field(request, 3);
    LocalDate businessDate = ctx.get(TxnContextKeys.BUSINESS_DATE);

    return new TranLogRow(
        businessDate == null ? LocalDate.now() : businessDate,
        mti(request),
        TRAN_TYPE_BY_PROCESSING_CODE.getOrDefault(processingCode, "PURCHASE"),
        processingCode == null ? "000000" : processingCode,
        ctx.<String>get(TxnContextKeys.ACQUIRER_ID),
        field(request, 41),
        field(request, 42),
        field(request, 11),
        field(request, 7),
        field(request, 37),
        amount == null ? 0L : amount,
        "704",
        cardId,
        status,
        responseCode,
        authCode,
        declineReason);
  }

  private static String field(ISOMsg msg, int number) {
    try {
      return msg.hasField(number) ? msg.getString(number) : null;
    } catch (Exception e) {
      return null;
    }
  }

  private static String mti(ISOMsg msg) {
    try {
      return msg.getMTI();
    } catch (Exception e) {
      throw new IllegalStateException("request has no MTI", e);
    }
  }
}
