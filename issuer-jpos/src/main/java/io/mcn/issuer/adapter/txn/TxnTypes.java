package io.mcn.issuer.adapter.txn;

import io.mcn.issuer.domain.TransactionType;
import io.mcn.issuer.domain.TransactionType.CustomerEffect;
import org.jpos.transaction.Context;

/** Reads the request's {@link TransactionType} back out of the jPOS {@link Context}. */
final class TxnTypes {
  private TxnTypes() {}

  /**
   * The type {@code ParseAndValidate} validated. A context without a processing code (a participant
   * driven directly by a test) is a purchase, the chain's original only type.
   */
  static TransactionType of(Context ctx) {
    String processingCode = ctx.get(TxnContextKeys.PROCESSING_CODE);
    if (processingCode == null) return TransactionType.PURCHASE;
    return TransactionType.fromProcessingCode(processingCode)
        .orElseThrow(
            () -> new IllegalStateException("unvalidated processing code " + processingCode));
  }

  static boolean debitsCustomer(Context ctx) {
    return of(ctx).customerEffect() == CustomerEffect.DEBIT;
  }
}
