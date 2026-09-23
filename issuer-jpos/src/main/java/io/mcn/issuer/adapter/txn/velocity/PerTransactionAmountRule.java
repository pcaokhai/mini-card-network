package io.mcn.issuer.adapter.txn.velocity;

import io.mcn.issuer.adapter.persistence.CardLimit;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import io.mcn.issuer.application.VelocityRule;
import java.util.Optional;

/** Declines RC 61 when a single transaction's amount exceeds the card's PER_TXN ceiling. */
public class PerTransactionAmountRule implements VelocityRule {
  private static final String TRAN_TYPE = "PURCHASE";

  private final CardLimitRepository cardLimitRepository;

  public PerTransactionAmountRule(CardLimitRepository cardLimitRepository) {
    this.cardLimitRepository = cardLimitRepository;
  }

  /** Reflective config-driven registration factory; see {@code CheckLimits#setConfiguration}. */
  public static PerTransactionAmountRule create(
      CardLimitRepository cardLimitRepository,
      VelocityCounterRepository velocityCounterRepository) {
    return new PerTransactionAmountRule(cardLimitRepository);
  }

  @Override
  public Optional<String> evaluate(long cardId, long amountMinorUnits) {
    for (CardLimit limit : cardLimitRepository.findApplicableLimits(cardId, TRAN_TYPE)) {
      if ("PER_TXN".equals(limit.period())
          && limit.maxAmount() != null
          && amountMinorUnits > limit.maxAmount()) {
        return Optional.of("61");
      }
    }
    return Optional.empty();
  }
}
