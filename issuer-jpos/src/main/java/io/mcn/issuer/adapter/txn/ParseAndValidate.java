package io.mcn.issuer.adapter.txn;

import io.mcn.issuer.domain.TransactionType;
import java.io.Serializable;
import java.time.LocalDate;
import java.util.Optional;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;

/**
 * Validates the parsed request against {@code docs/03} §6 (known processing codes) and DE 4 (amount
 * must be positive). Declines RC 12 (unsupported processing code) or RC 13 (invalid amount);
 * otherwise extracts the fields every later participant needs into the {@link Context}.
 */
public class ParseAndValidate implements TransactionParticipant {

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);

    try {
      String processingCode = request.getString(3);
      Optional<TransactionType> type = TransactionType.fromProcessingCode(processingCode);
      if (type.isEmpty()) {
        ctx.put(TxnContextKeys.RESPONSE_CODE, "12");
        return ABORTED;
      }

      // A balance inquiry asks for no amount: the gateway leaves DE 4 out (docs/03 §4 C6).
      boolean movesMoney = type.get().customerEffect() != TransactionType.CustomerEffect.NONE;
      long amount = movesMoney || request.hasField(4) ? Long.parseLong(request.getString(4)) : 0L;
      if (movesMoney && amount <= 0) {
        ctx.put(TxnContextKeys.RESPONSE_CODE, "13");
        return ABORTED;
      }

      ctx.put(TxnContextKeys.PROCESSING_CODE, processingCode);
      ctx.put(TxnContextKeys.AMOUNT, amount);
      // ponytail: no cutover/business-date service yet (separate epic) - wall-clock date is
      // accurate for this story's scope (expiry check, dedupe key).
      ctx.put(TxnContextKeys.BUSINESS_DATE, LocalDate.now());
      if (request.hasField(32)) {
        ctx.put(TxnContextKeys.ACQUIRER_ID, request.getString(32));
      }
      if (request.hasField(2)) {
        ctx.put(TxnContextKeys.PAN, request.getString(2));
      }
      return PREPARED;
    } catch (Exception e) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "30");
      return ABORTED;
    }
  }
}
