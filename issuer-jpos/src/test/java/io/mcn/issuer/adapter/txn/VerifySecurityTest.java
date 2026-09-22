package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;

import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class VerifySecurityTest {

  @Test
  void prepareAlwaysReturnsPrepared() {
    // no-op until E5 (MCN-501+) - this test pins that contract so a future change to real PIN/MAC
    // verification fails here first, intentionally, instead of drifting silently.
    Context ctx = new Context();

    int result = new VerifySecurity().prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
  }
}
