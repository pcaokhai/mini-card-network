package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.ReversalWithoutOriginalRepository;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.TranLogRow;
import java.io.Serializable;
import java.time.LocalDate;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;
import org.jpos.util.Destroyable;

/**
 * Looks up {@code tran_log}'s dedupe key (docs/05 §6, {@code uq_tran_dedupe}). A hit is not an
 * abort: it flows through unchanged so {@code Respond} can replay the stored outcome verbatim
 * (docs/03 §7.5) instead of re-processing the request.
 *
 * <p>MCN-402: before that lookup, also checks whether a reversal already recorded {@code
 * reversal_without_original} for this exact request's own MTI/STAN/DE7/acquirer identity (docs/03
 * §7.3: "if the original is not found, ... if the original arrives later it is declined with RC 94
 * and not posted"). A hit there declines RC 94 directly - {@code CheckCard}/{@code CheckLimits}
 * don't guard on an already-set {@code RESPONSE_CODE} the way {@code Authorize} does, but they only
 * ever overwrite it on their own failure conditions, so a valid card/limits leaves RC 94 intact
 * through to {@code Authorize}'s guard, which skips debiting.
 */
public class Deduplicate implements TransactionParticipant, Configurable, Destroyable {

  private TranLogRepository tranLogRepository;
  private ReversalWithoutOriginalRepository reversalWithoutOriginalRepository;
  private HikariDataSource dataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public Deduplicate() {}

  public Deduplicate(TranLogRepository tranLogRepository) {
    this(tranLogRepository, null);
  }

  public Deduplicate(
      TranLogRepository tranLogRepository,
      ReversalWithoutOriginalRepository reversalWithoutOriginalRepository) {
    this.tranLogRepository = tranLogRepository;
    this.reversalWithoutOriginalRepository = reversalWithoutOriginalRepository;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.dataSource = TxnDataSource.fromConfig(cfg);
    this.tranLogRepository = new TranLogRepository(dataSource);
    this.reversalWithoutOriginalRepository = new ReversalWithoutOriginalRepository(dataSource);
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
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);
    String acquirerId = ctx.get(TxnContextKeys.ACQUIRER_ID);
    String rawMti;
    try {
      rawMti = request.getMTI();
    } catch (org.jpos.iso.ISOException e) {
      throw new IllegalStateException("request has no MTI", e);
    }
    String mti = normalizeMti(rawMti);

    if (reversalWithoutOriginalRepository != null
        && reversalWithoutOriginalRepository.findByKey(
            rawMti, request.getString(11), request.getString(7), padAcquirer(acquirerId))) {
      ctx.put(TxnContextKeys.IS_DUPLICATE, false);
      ctx.put(TxnContextKeys.RESPONSE_CODE, "94");
      return PREPARED;
    }

    var found =
        tranLogRepository.findByDedupeKey(
            acquirerId,
            request.getString(41),
            request.getString(11),
            request.getString(7),
            mti,
            LocalDate.now());

    if (found.isPresent()) {
      TranLogRow stored = found.get();
      ctx.put(TxnContextKeys.IS_DUPLICATE, true);
      // docs/03 §7.5: replay the stored answer - RC, auth code and a balance inquiry's DE 54.
      ctx.put(TxnContextKeys.RESPONSE_CODE, stored.responseCode());
      if (stored.authCode() != null) ctx.put(TxnContextKeys.AUTH_CODE, stored.authCode());
      if (stored.balance() != null) {
        ctx.put(TxnContextKeys.BALANCE, stored.balance());
        ctx.put(TxnContextKeys.BALANCE_CURRENCY, stored.currency());
      }
    } else {
      ctx.put(TxnContextKeys.IS_DUPLICATE, false);
    }
    return PREPARED;
  }

  /** MTI x21 (repeat) is treated as x20 for dedupe purposes, per docs/03 §5. */
  private static String normalizeMti(String mti) {
    if (mti.charAt(3) == '1') {
      return mti.substring(0, 3) + "0";
    }
    return mti;
  }

  /**
   * DE 90's acquirer sub-field is fixed-width 11, matching {@code
   * reversal_without_original.original_acquirer} (CHAR(11)) - pads field 32's shorter acquirer id
   * the same way so a purchase's own identity matches what a reversal recorded for it.
   */
  private static String padAcquirer(String acquirerId) {
    return String.format("%-11s", acquirerId);
  }
}
