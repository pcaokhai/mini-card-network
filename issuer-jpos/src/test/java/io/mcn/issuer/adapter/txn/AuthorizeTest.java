package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;

import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class AuthorizeTest {

  @Test
  void noPriorDeclineFallsThroughToPendingLedgerNotApproval() {
    Context ctx = new Context(); // nothing set RESPONSE_CODE yet - would otherwise approve

    int result = new Authorize().prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED); // not aborted - flows to LogAndOutbox/Respond
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("96");
    assertThat(ctx.<String>get(TxnContextKeys.DECLINE_REASON))
        .isEqualTo("ledger posting not implemented until MCN-302b");
  }

  @Test
  void priorDeclineIsPreserved() {
    Context ctx = new Context();
    ctx.put(TxnContextKeys.RESPONSE_CODE, "61");
    ctx.put(TxnContextKeys.DECLINE_REASON, "limit exceeded");

    new Authorize().prepare(1L, ctx);

    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("61"); // unchanged
  }
}
