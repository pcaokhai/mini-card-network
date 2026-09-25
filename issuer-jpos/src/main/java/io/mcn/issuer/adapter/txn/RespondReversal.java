package io.mcn.issuer.adapter.txn;

import java.io.Serializable;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.ISOSource;
import org.jpos.transaction.AbortParticipant;
import org.jpos.transaction.Context;

/**
 * Always sends 0430 (docs/03 §7.3: "Advices cannot be declined") - never inspects {@code
 * RESPONSE_CODE}/{@code IS_DUPLICATE}, unlike {@code Respond}. DE 39 is "00" once the reversal is
 * recorded and "96" when the transaction aborted, so the acquirer repeats it. {@code
 * setResponseMTI()} maps both 0420 and 0421 (repeat) to 0430. Implements {@link AbortParticipant}
 * for the same reason {@code Respond} does: every member of an aborting transaction still needs its
 * response sent.
 */
public class RespondReversal implements AbortParticipant {

  private static final String ACKNOWLEDGED = "00";
  private static final String SYSTEM_MALFUNCTION = "96";

  @Override
  public int prepare(long id, Serializable context) {
    return PREPARED;
  }

  @Override
  public void commit(long id, Serializable context) {
    send(context, ACKNOWLEDGED);
  }

  @Override
  public void abort(long id, Serializable context) {
    // Not recorded, so still owed: anything but "00" makes the acquirer's SAF repeat it.
    send(context, SYSTEM_MALFUNCTION);
  }

  private void send(Serializable context, String responseCode) {
    Context ctx = (Context) context;
    ISOSource source = ctx.get(TxnContextKeys.SOURCE);
    if (source == null) {
      return;
    }
    try {
      source.send(buildResponse(ctx, responseCode));
    } catch (ISOException | java.io.IOException e) {
      throw new IllegalStateException("failed to send reversal response", e);
    }
  }

  ISOMsg buildResponse(Context ctx) throws ISOException {
    return buildResponse(ctx, ACKNOWLEDGED);
  }

  private ISOMsg buildResponse(Context ctx, String responseCode) throws ISOException {
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);
    ISOMsg response = (ISOMsg) request.clone();
    response.setResponseMTI();
    // The clone carries the 0420's reason code in DE 39; the 0430's DE 39 is the acknowledgement
    // (docs/03 §3), and the acquirer only treats "00" as delivered.
    response.set(39, responseCode);
    return response;
  }
}
