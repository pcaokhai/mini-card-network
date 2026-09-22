package io.mcn.issuer.adapter.txn;

import java.io.Serializable;
import java.util.Set;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;

/**
 * Validates the parsed request against {@code docs/03} §6 (known processing codes) and DE 4
 * (amount must be positive). Declines RC 12 (unsupported processing code) or RC 13 (invalid
 * amount); otherwise extracts the fields every later participant needs into the {@link Context}.
 */
public class ParseAndValidate implements TransactionParticipant {

  private static final Set<String> KNOWN_PROCESSING_CODES =
      Set.of("000000", "010000", "200000", "310000");

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);

    try {
      String processingCode = request.getString(3);
      if (!KNOWN_PROCESSING_CODES.contains(processingCode)) {
        ctx.put(TxnContextKeys.RESPONSE_CODE, "12");
        return ABORTED;
      }

      long amount = Long.parseLong(request.getString(4));
      if (amount <= 0) {
        ctx.put(TxnContextKeys.RESPONSE_CODE, "13");
        return ABORTED;
      }

      ctx.put(TxnContextKeys.PROCESSING_CODE, processingCode);
      ctx.put(TxnContextKeys.AMOUNT, amount);
      if (request.hasField(32)) {
        ctx.put(TxnContextKeys.ACQUIRER_ID, request.getString(32));
      }
      return PREPARED;
    } catch (Exception e) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "30");
      return ABORTED;
    }
  }
}
