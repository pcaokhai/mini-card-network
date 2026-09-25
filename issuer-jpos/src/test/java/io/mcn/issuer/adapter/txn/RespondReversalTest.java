package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;

import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class RespondReversalTest {

  @Test
  void buildResponse_always0430ForA0420_MCN_402_AC1() throws Exception {
    ISOMsg request = new ISOMsg("0420");
    request.set(11, "000123");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);

    ISOMsg response = new RespondReversal().buildResponse(ctx);

    assertThat(response.getMTI()).isEqualTo("0430");
  }

  @Test
  void buildResponse_always0430ForA0421Repeat_MCN_402_AC3() throws Exception {
    ISOMsg request = new ISOMsg("0421");
    request.set(11, "000123");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);

    ISOMsg response = new RespondReversal().buildResponse(ctx);

    assertThat(response.getMTI()).isEqualTo("0430");
  }

  @Test
  void buildResponse_ignoresResponseCodeContextKey_MCN_402_AC1() throws Exception {
    ISOMsg request = new ISOMsg("0420");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.RESPONSE_CODE, "94"); // must never be read by RespondReversal

    ISOMsg response = new RespondReversal().buildResponse(ctx);

    assertThat(response.getMTI()).isEqualTo("0430");
  }

  /** docs/03 §3/§7.3: 0430 DE 39 is the acknowledgement, not an echo of the 0420's reason code. */
  @Test
  void buildResponse_acknowledgesWith00RatherThanEchoingTheReason__MCN_401() throws Exception {
    org.jpos.iso.ISOMsg request = new org.jpos.iso.ISOMsg("0420");
    request.set(39, "17");
    org.jpos.transaction.Context ctx = new org.jpos.transaction.Context();
    ctx.put(TxnContextKeys.REQUEST, request);

    org.jpos.iso.ISOMsg response = new RespondReversal().buildResponse(ctx);

    assertThat(response.getString(39)).isEqualTo("00");
  }

  /** A reversal the issuer failed to record is still owed: never acknowledge it with "00". */
  @Test
  void abort_answers96SoTheAcquirerRepeats__MCN_401() throws Exception {
    ISOMsg request = new ISOMsg("0420");
    request.set(39, "68");
    var sent = new java.util.ArrayList<ISOMsg>();
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.SOURCE, recordingSource(sent));

    new RespondReversal().abort(0, ctx);

    assertThat(sent).singleElement().satisfies(r -> assertThat(r.getString(39)).isEqualTo("96"));
  }

  private static org.jpos.iso.ISOSource recordingSource(java.util.List<ISOMsg> sent) {
    return new org.jpos.iso.ISOSource() {
      @Override
      public void send(ISOMsg m) {
        sent.add(m);
      }

      @Override
      public boolean isConnected() {
        return true;
      }
    };
  }
}
