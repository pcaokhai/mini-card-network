package io.mcn.issuer.adapter.txn;

import java.io.Serializable;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.ISOSource;
import org.jpos.transaction.AbortParticipant;
import org.jpos.transaction.Context;

/**
 * Builds the 0210 response and sends it via the {@link ISOSource} stored in the context. A
 * duplicate replays the stored response's DE 38/39/4/54 verbatim (docs/03 §7.5) instead of
 * rebuilding them. Implements {@link AbortParticipant} and overrides {@code abort()} for the same
 * reason as {@code LogAndOutbox}: a decline earlier in the chain makes the whole transaction abort,
 * and every declined request still needs a response sent.
 */
public class Respond implements AbortParticipant {

  @Override
  public int prepare(long id, Serializable context) {
    return PREPARED;
  }

  @Override
  public void commit(long id, Serializable context) {
    send(context);
  }

  @Override
  public void abort(long id, Serializable context) {
    send(context);
  }

  private void send(Serializable context) {
    Context ctx = (Context) context;
    ISOSource source = ctx.get(TxnContextKeys.SOURCE);
    if (source == null) {
      return;
    }
    try {
      source.send(buildResponse(ctx));
    } catch (ISOException | java.io.IOException e) {
      throw new IllegalStateException("failed to send authorization response", e);
    }
  }

  ISOMsg buildResponse(Context ctx) throws ISOException {
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);
    ISOMsg response = (ISOMsg) request.clone();
    response.setResponseMTI();

    if (Boolean.TRUE.equals(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE))) {
      ISOMsg stored = ctx.get(TxnContextKeys.STORED_RESPONSE);
      if (stored != null) {
        copyIfPresent(stored, response, 4);
        copyIfPresent(stored, response, 38);
        copyIfPresent(stored, response, 39);
        copyIfPresent(stored, response, 54);
        return response;
      }
    }

    response.set(39, ctx.<String>get(TxnContextKeys.RESPONSE_CODE));
    return response;
  }

  private static void copyIfPresent(ISOMsg from, ISOMsg to, int field) throws ISOException {
    if (from.hasField(field)) {
      to.set(field, from.getString(field));
    }
  }
}
