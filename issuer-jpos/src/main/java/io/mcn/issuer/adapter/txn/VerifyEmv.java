package io.mcn.issuer.adapter.txn;

import io.mcn.issuer.adapter.crypto.ArqcSimulator;
import io.mcn.issuer.adapter.crypto.EmvTlvParser;
import io.mcn.issuer.adapter.persistence.CardRepository;
import java.io.Serializable;
import java.math.BigInteger;
import java.util.Map;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;

/**
 * Parses DE 55 (EMV/chip data, docs/03 §11) when present, rejects malformed TLV with RC 30,
 * enforces the ATC replay guard (RC 05, MCN-602-AC2) and verifies the simulated ARQC, storing the
 * simulated ARPC in {@link TxnContextKeys#EMV_ARPC} for {@link Respond} to place in tag 91. Runs
 * after {@code CheckCard} and before {@code VerifySecurity} in the purchase chain: EMV format/replay
 * is a card-data validation concern, checked before the PIN/MAC layer. No-op (returns {@code
 * PREPARED}) when DE 55 is absent - manual/magstripe entry has no EMV data to verify.
 */
public class VerifyEmv implements TransactionParticipant {

  private final ArqcSimulator arqcSimulator;
  private final CardRepository cardRepository;

  public VerifyEmv(ArqcSimulator arqcSimulator, CardRepository cardRepository) {
    this.arqcSimulator = arqcSimulator;
    this.cardRepository = cardRepository;
  }

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);

    if (!request.hasField(55)) {
      return PREPARED;
    }

    Map<String, byte[]> tags;
    try {
      tags = EmvTlvParser.parse(request.getBytes(55));
    } catch (EmvTlvParser.MalformedTlvException e) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "30");
      return ABORTED;
    }

    long cardId = ctx.get(TxnContextKeys.CARD_ID);
    String pan = ctx.get(TxnContextKeys.PAN);

    byte[] atcBytes = tags.get("9F36");
    int atc = atcBytes == null ? 0 : new BigInteger(1, atcBytes).intValue();
    if (!cardRepository.updateEmvLastAtcIfIncreasing(cardId, atc)) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "05");
      return ABORTED;
    }

    byte[] arqc = tags.get("9F26");
    byte[] unpredictableNumber = tags.get("9F37");
    if (arqc == null || !arqcSimulator.verifyArqc(arqc, pan, atc, unpredictableNumber)) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "05");
      return ABORTED;
    }

    ctx.put(TxnContextKeys.EMV_ARPC, arqcSimulator.computeArpc(arqc));
    return PREPARED;
  }
}
