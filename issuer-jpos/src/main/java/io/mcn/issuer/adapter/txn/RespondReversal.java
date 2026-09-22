package io.mcn.issuer.adapter.txn;

import java.io.Serializable;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.ISOSource;
import org.jpos.transaction.AbortParticipant;
import org.jpos.transaction.Context;

/**
 * Always sends 0430 (docs/03 §7.3: "Advices cannot be declined") - never inspects {@code
 * RESPONSE_CODE}/{@code IS_DUPLICATE}, unlike {@code Respond}. {@code setResponseMTI()} maps both
 * 0420 and 0421 (repeat) to 0430. Implements {@link AbortParticipant} for the same reason {@code
 * Respond} does: every member of an aborting transaction still needs its response sent.
 */
public class RespondReversal implements AbortParticipant {

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
      throw new IllegalStateException("failed to send reversal response", e);
    }
  }

  ISOMsg buildResponse(Context ctx) throws ISOException {
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);
    ISOMsg response = (ISOMsg) request.clone();
    response.setResponseMTI();
    return response;
  }
}
