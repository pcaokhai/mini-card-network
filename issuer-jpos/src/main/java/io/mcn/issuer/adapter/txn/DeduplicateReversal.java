package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
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
 * Catches an exact byte-identical resend of the same 0420/0421 - matching the reversal's *own*
 * MTI/STAN/DE7/acquirer (not DE 90's original-transaction identity, docs/plans/MCN-402.md Ruling
 * 3). This is distinct from {@code LocateAndReverse}'s own idempotency check, which recognizes a
 * semantic repeat of the same underlying reversal via DE 90; both checks are needed.
 */
public class DeduplicateReversal implements TransactionParticipant, Configurable, Destroyable {

  private TranLogRepository tranLogRepository;
  private HikariDataSource dataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public DeduplicateReversal() {}

  public DeduplicateReversal(TranLogRepository tranLogRepository) {
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
      throw new IllegalStateException("reversal request has no MTI", e);
    }
    LocalDate businessDate = ctx.get(TxnContextKeys.BUSINESS_DATE);

    var found =
        tranLogRepository.findByDedupeKey(
            acquirerId,
            request.getString(41),
            request.getString(11),
            request.getString(7),
            mti,
            businessDate == null ? LocalDate.now() : businessDate);

    ctx.put(TxnContextKeys.IS_DUPLICATE, found.isPresent());
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
