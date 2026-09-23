package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;

import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class RespondTest {

  @Test
  void buildsResponseWithRc14ForUnknownCard() throws Exception {
    ISOMsg request = new ISOMsg("0200");
    request.set(11, "000001");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.RESPONSE_CODE, "14");

    ISOMsg response = new Respond().buildResponse(ctx);

    assertThat(response.getMTI()).isEqualTo("0210");
    assertThat(response.getString(39)).isEqualTo("14");
    assertThat(response.getString(11)).isEqualTo("000001"); // DE 11 echoed
  }

  @Test
  void duplicateReplaysStoredResponseVerbatim() throws Exception {
    ISOMsg request = new ISOMsg("0200");
    ISOMsg stored = new ISOMsg("0210");
    stored.set(39, "00");
    stored.set(38, "123456");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.IS_DUPLICATE, true);
    ctx.put(TxnContextKeys.STORED_RESPONSE, stored);

    ISOMsg response = new Respond().buildResponse(ctx);

    assertThat(response.getString(39)).isEqualTo("00");
    assertThat(response.getString(38)).isEqualTo("123456");
  }

  @Test
  void setsTag91ArpcOnApprovedChipTransaction__MCN_602_AC3() throws Exception {
    ISOMsg request = new ISOMsg("0200");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.RESPONSE_CODE, "00");
    byte[] arpc = new byte[] {1, 2, 3, 4, 5, 6, 7, 8};
    ctx.put(TxnContextKeys.EMV_ARPC, arpc);

    ISOMsg response = new Respond().buildResponse(ctx);

    byte[] de55 = response.getBytes(55);
    assertThat(de55[0]).isEqualTo((byte) 0x91);
    assertThat(de55[1]).isEqualTo((byte) arpc.length);
  }

  @Test
  void omitsTag91ArpcOnDeclinedTransaction() throws Exception {
    ISOMsg request = new ISOMsg("0200");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.RESPONSE_CODE, "05");
    ctx.put(TxnContextKeys.EMV_ARPC, new byte[] {1, 2, 3, 4, 5, 6, 7, 8});

    ISOMsg response = new Respond().buildResponse(ctx);

    assertThat(response.hasField(55)).isFalse();
  }
}
