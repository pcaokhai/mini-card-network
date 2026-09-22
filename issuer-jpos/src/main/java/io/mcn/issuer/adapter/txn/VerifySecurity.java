package io.mcn.issuer.adapter.txn;

import java.io.Serializable;
import org.jpos.transaction.TransactionParticipant;

/**
 * Documented no-op until E5 (MCN-501+ adds real PIN/MAC/ARQC verification). Wired into the chain
 * now so ordering is correct end to end; unconditionally prepares.
 */
public class VerifySecurity implements TransactionParticipant {

  @Override
  public int prepare(long id, Serializable context) {
    return PREPARED;
  }
}
