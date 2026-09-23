package io.mcn.issuer.adapter.txn.velocity;

import io.mcn.issuer.adapter.persistence.CardLimit;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRow;
import io.mcn.issuer.application.VelocityRule;
import java.time.LocalDate;
import java.util.Optional;

/**
 * Declines RC 61 when today's posted amount plus this transaction would exceed the card's DAILY
 * amount ceiling.
 */
public class DailyAmountRule implements VelocityRule {
  private static final String TRAN_TYPE = "PURCHASE";

  private final CardLimitRepository cardLimitRepository;
  private final VelocityCounterRepository velocityCounterRepository;

  public DailyAmountRule(
      CardLimitRepository cardLimitRepository,
      VelocityCounterRepository velocityCounterRepository) {
    this.cardLimitRepository = cardLimitRepository;
    this.velocityCounterRepository = velocityCounterRepository;
  }

  /** Reflective config-driven registration factory; see {@code CheckLimits#setConfiguration}. */
  public static DailyAmountRule create(
      CardLimitRepository cardLimitRepository,
      VelocityCounterRepository velocityCounterRepository) {
    return new DailyAmountRule(cardLimitRepository, velocityCounterRepository);
  }

  @Override
  public Optional<String> evaluate(long cardId, long amountMinorUnits) {
    for (CardLimit limit : cardLimitRepository.findApplicableLimits(cardId, TRAN_TYPE)) {
      if ("DAILY".equals(limit.period()) && limit.maxAmount() != null) {
        long amountSoFar =
            velocityCounterRepository
                .findDaily(cardId, TRAN_TYPE, LocalDate.now())
                .map(VelocityCounterRow::txnAmount)
                .orElse(0L);
        if (amountSoFar + amountMinorUnits > limit.maxAmount()) {
          return Optional.of("61");
        }
      }
    }
    return Optional.empty();
  }
}
