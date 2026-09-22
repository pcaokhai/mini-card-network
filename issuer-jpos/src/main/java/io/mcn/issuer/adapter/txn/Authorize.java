package io.mcn.issuer.adapter.txn;

import java.io.Serializable;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;

/**
 * Ruling (docs/plans/MCN-302a.md): a would-be approval (nothing upstream declined) sets RC 96
 * with an honest placeholder reason rather than fabricating RC 00 - real ledger posting is
 * MCN-302b. Never overwrites a decline an earlier participant already set.
 */
public class Authorize implements TransactionParticipant {

  static final String LEDGER_NOT_IMPLEMENTED_REASON =
      "ledger posting not implemented until MCN-302b";

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    if (ctx.<String>get(TxnContextKeys.RESPONSE_CODE) == null) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "96");
      ctx.put(TxnContextKeys.DECLINE_REASON, LEDGER_NOT_IMPLEMENTED_REASON);
    }
    return PREPARED;
  }
}
