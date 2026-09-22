package io.mcn.issuer.adapter.txn;

import io.mcn.issuer.adapter.persistence.CardLimit;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import java.io.Serializable;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;

/**
 * Compares the transaction amount/count against {@code card_limit} ceilings: RC 61 for an
 * amount-limit breach, RC 65 for a frequency-limit breach (docs/03 §8 distinguishes the two).
 * Account balance (RC 51) is 302b's scope per the plan's Ruling.
 */
public class CheckLimits implements TransactionParticipant, Configurable {

  private static final String PURCHASE_TRAN_TYPE = "PURCHASE";

  private CardLimitRepository cardLimitRepository;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public CheckLimits() {}

  public CheckLimits(CardLimitRepository cardLimitRepository) {
    this.cardLimitRepository = cardLimitRepository;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.cardLimitRepository = new CardLimitRepository(TxnDataSource.fromConfig(cfg));
  }

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    long cardId = ctx.get(TxnContextKeys.CARD_ID);
    long amount = ctx.get(TxnContextKeys.AMOUNT);

    for (CardLimit limit : cardLimitRepository.findApplicableLimits(cardId, PURCHASE_TRAN_TYPE)) {
      if ("PER_TXN".equals(limit.period()) && limit.maxAmount() != null && amount > limit.maxAmount()) {
        ctx.put(TxnContextKeys.RESPONSE_CODE, "61");
        return ABORTED;
      }
      if ("DAILY".equals(limit.period()) && limit.maxCount() != null) {
        int countSoFar = cardLimitRepository.countToday(cardId, PURCHASE_TRAN_TYPE);
        if (countSoFar >= limit.maxCount()) {
          ctx.put(TxnContextKeys.RESPONSE_CODE, "65");
          return ABORTED;
        }
      }
      if ("DAILY".equals(limit.period()) && limit.maxAmount() != null && amount > limit.maxAmount()) {
        ctx.put(TxnContextKeys.RESPONSE_CODE, "61");
        return ABORTED;
      }
    }
    return PREPARED;
  }
}
