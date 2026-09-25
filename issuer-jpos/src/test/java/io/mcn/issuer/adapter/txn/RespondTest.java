package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;

import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import java.util.HexFormat;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.packager.GenericPackager;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class RespondTest {

  private static final String LMK_HEX =
      "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
  private static final byte[] ZAK = HexFormat.of().parseHex("3132333435363738393a3b3c3d3e3f40");

  private static ISOMsg purchaseRequest() throws Exception {
    ISOMsg request = new ISOMsg("0200");
    request.setPackager(new GenericPackager("src/dist/cfg/iso87ascii.xml"));
    request.set(3, "000000");
    request.set(4, "000000250000");
    request.set(7, "0921073208");
    request.set(11, "000123");
    request.set(37, "626514000123");
    request.set(41, "00000042");
    request.set(42, "GOCPHO000000001");
    request.set(49, "704");
    return request;
  }

  /** The gateway rejects any 0210 without a valid DE 64 as a MAC failure (RC 96), docs/03 §11. */
  @Test
  void signsTheResponseWithARetailMacUnderTheZak__MCN_502() throws Exception {
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, purchaseRequest());
    ctx.put(TxnContextKeys.RESPONSE_CODE, "00");
    ctx.put(TxnContextKeys.AUTH_CODE, "123456");
    JCESecurityModule hsm = new JCESecurityModule(LMK_HEX);

    ISOMsg response = new Respond(hsm, ZAK).signedResponse(ctx);

    assertThat(response.hasField(64)).isTrue();
    ISOMsg unsigned = (ISOMsg) response.clone();
    unsigned.unset(64);
    assertThat(response.getBytes(64)).isEqualTo(hsm.computeMac(unsigned.pack(), ZAK));
  }

  @Test
  void signsReplayedDuplicatesToo__MCN_502() throws Exception {
    ISOMsg stored = new ISOMsg("0210");
    stored.set(39, "00");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, purchaseRequest());
    ctx.put(TxnContextKeys.IS_DUPLICATE, true);
    ctx.put(TxnContextKeys.STORED_RESPONSE, stored);

    ISOMsg response = new Respond(new JCESecurityModule(LMK_HEX), ZAK).signedResponse(ctx);

    assertThat(response.hasField(64)).isTrue();
  }

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
