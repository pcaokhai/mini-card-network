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
    request.setPackager(new org.jpos.iso.packager.GenericPackager("src/dist/cfg/iso87ascii.xml"));
    request.set(39, "68");
    var sent = new java.util.ArrayList<ISOMsg>();
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.SOURCE, recordingSource(sent));

    new RespondReversal(
            new io.mcn.issuer.adapter.crypto.JCESecurityModule(LMK_HEX),
            FixedSessionKeys.of(ZAK, ZAK))
        .abort(0, ctx);

    assertThat(sent)
        .singleElement()
        .satisfies(
            r -> {
              assertThat(r.getString(39)).isEqualTo("96");
              assertThat(r.hasField(64)).isTrue(); // the "not recorded" answer is MACed too
            });
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

  private static final String LMK_HEX =
      "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
  private static final byte[] ZAK =
      java.util.HexFormat.of().parseHex("3132333435363738393a3b3c3d3e3f40");

  /**
   * docs/03 §4: 64/128 is mandatory on the 0430. The echoed DE 90 sets the secondary bitmap, so the
   * MAC goes in DE 128 (as the gateway MACs its 0420), under the ACTIVE ZAK - never the acquirer's
   * own 0420 MAC echoed back.
   */
  @Test
  @org.junit.jupiter.api.DisplayName(
      "0430 MAC: the reversal ack is MACed in DE 128 under the active ZAK, not the 0420's echoed")
  void should_macThe0430InDe128UnderTheActiveZak() throws Exception {
    ISOMsg request = new ISOMsg("0420");
    request.setPackager(new org.jpos.iso.packager.GenericPackager("src/dist/cfg/iso87ascii.xml"));
    request.set(3, "000000");
    request.set(4, "000000010000");
    request.set(7, "0922120500");
    request.set(11, "000123");
    request.set(39, "17");
    request.set(90, "0200000122092212000000000970499" + "0".repeat(11));
    request.set(128, "0123456789ABCDEF"); // the acquirer's MAC over its 0420
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    var hsm = new io.mcn.issuer.adapter.crypto.JCESecurityModule(LMK_HEX);

    ISOMsg response =
        new RespondReversal(hsm, FixedSessionKeys.of(ZAK, ZAK)).signedResponse(ctx, "00");

    assertThat(response.hasField(64)).isFalse();
    ISOMsg unsigned = (ISOMsg) response.clone();
    unsigned.unset(128);
    assertThat(response.getBytes(128)).isEqualTo(hsm.computeMac(unsigned.pack(), ZAK));
    assertThat(response.getString(128)).isNotEqualToIgnoringCase("0123456789ABCDEF");
  }
}
