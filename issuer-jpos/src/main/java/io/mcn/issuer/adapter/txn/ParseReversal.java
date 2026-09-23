package io.mcn.issuer.adapter.txn;

import java.io.Serializable;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;

/**
 * Decodes DE 90 (n 42, docs/03 §7.3 layout: original MTI (4) + original STAN (6) + original DE 7
 * (10) + original acquirer ID (11) + forwarding institution ID (11, unused in v1)) into the {@link
 * Context} for {@code DeduplicateReversal}/{@code LocateAndReverse}, plus DE 39 as the reversal
 * reason. Always prepares - a reversal is never declined for a malformed DE 90 (advices can't be
 * declined); a request this malformed would already have failed packager unpacking upstream.
 */
public class ParseReversal implements TransactionParticipant {

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);

    String originalData = request.getString(90);
    ctx.put(TxnContextKeys.ORIGINAL_MTI, originalData.substring(0, 4));
    ctx.put(TxnContextKeys.ORIGINAL_STAN, originalData.substring(4, 10));
    ctx.put(TxnContextKeys.ORIGINAL_DE7, originalData.substring(10, 20));
    // trim(): tran_log.acquirer_id (what LocateAndReverse's findByReversalKey matches against)
    // is stored unpadded, unlike this DE 90 sub-field's fixed width.
    ctx.put(TxnContextKeys.ORIGINAL_ACQUIRER, originalData.substring(20, 31).trim());

    if (request.hasField(39)) {
      ctx.put(TxnContextKeys.REVERSAL_REASON, request.getString(39));
    }
    if (request.hasField(32)) {
      ctx.put(TxnContextKeys.ACQUIRER_ID, request.getString(32));
    }
    ctx.put(TxnContextKeys.BUSINESS_DATE, java.time.LocalDate.now());
    return PREPARED;
  }
}
