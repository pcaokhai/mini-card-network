package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
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
 */
public class Deduplicate implements TransactionParticipant, Configurable, Destroyable {

  private TranLogRepository tranLogRepository;
  private HikariDataSource dataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public Deduplicate() {}

  public Deduplicate(TranLogRepository tranLogRepository) {
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
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);
    String acquirerId = ctx.get(TxnContextKeys.ACQUIRER_ID);
    String mti;
    try {
      mti = normalizeMti(request.getMTI());
    } catch (org.jpos.iso.ISOException e) {
      throw new IllegalStateException("request has no MTI", e);
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
      ctx.put(TxnContextKeys.RESPONSE_CODE, stored.responseCode());
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
}
