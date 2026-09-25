package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;

import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class ParseReversalTest {

  @Test
  void prepare_extractsDe90ComponentsIntoContext_MCN_402_AC1() throws Exception {
    ISOMsg request = new ISOMsg("0420");
    // DE 90: original MTI(4) + original STAN(6) + original DE7(10) + original acquirer(11) + fwd
    // inst(11)
    request.set(90, "0200" + "000123" + "0922140000" + "970499     " + "00000000000");
    request.set(39, "68");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);

    new ParseReversal().prepare(0, ctx);

    assertThat(ctx.<String>get(TxnContextKeys.ORIGINAL_MTI)).isEqualTo("0200");
    assertThat(ctx.<String>get(TxnContextKeys.ORIGINAL_STAN)).isEqualTo("000123");
    assertThat(ctx.<String>get(TxnContextKeys.ORIGINAL_DE7)).isEqualTo("0922140000");
    assertThat(ctx.<String>get(TxnContextKeys.ORIGINAL_ACQUIRER)).isEqualTo("970499");
    assertThat(ctx.<String>get(TxnContextKeys.REVERSAL_REASON)).isEqualTo("68");
  }

  /**
   * docs/03 §7.3: DE 90's acquirer ID is right-justified and zero-filled, as the gateway sends it.
   */
  @Test
  void prepare_stripsTheZeroFillFromTheAcquirerId__MCN_401() throws Exception {
    ISOMsg request = new ISOMsg("0420");
    request.set(90, "0200" + "000124" + "0921073244" + "00000970499" + "00000000000");
    request.set(39, "68");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);

    new ParseReversal().prepare(0, ctx);

    assertThat(ctx.<String>get(TxnContextKeys.ORIGINAL_ACQUIRER)).isEqualTo("970499");
  }
}
