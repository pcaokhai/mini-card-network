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
}
