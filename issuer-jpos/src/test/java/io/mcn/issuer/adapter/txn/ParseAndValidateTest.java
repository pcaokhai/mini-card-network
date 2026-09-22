package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.ABORTED;
import static org.jpos.transaction.TransactionConstants.PREPARED;

import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class ParseAndValidateTest {

  @Test
  void invalidAmountAbortsWithRc13() throws Exception {
    ISOMsg msg = new ISOMsg("0200");
    msg.set(3, "000000");
    msg.set(4, "000000000000"); // zero amount - invalid per RC 13
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, msg);

    int result = new ParseAndValidate().prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("13");
  }

  @Test
  void unsupportedProcessingCodeAbortsWithRc12() throws Exception {
    ISOMsg msg = new ISOMsg("0200");
    msg.set(3, "999999");
    msg.set(4, "000000010000");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, msg);

    int result = new ParseAndValidate().prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("12");
  }

  @Test
  void validPurchaseIsPrepared() throws Exception {
    ISOMsg msg = new ISOMsg("0200");
    msg.set(3, "000000");
    msg.set(4, "000000010000");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, msg);

    int result = new ParseAndValidate().prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
    assertThat(ctx.<Long>get(TxnContextKeys.AMOUNT)).isEqualTo(10000L);
  }
}
