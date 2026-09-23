package io.mcn.issuer.adapter.txn.velocity;

import io.mcn.issuer.adapter.persistence.CardLimit;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRow;
import io.mcn.issuer.application.VelocityRule;
import java.time.LocalDate;
import java.util.Optional;

/** Declines RC 65 when today's transaction count is already at the card's DAILY count ceiling. */
public class DailyCountRule implements VelocityRule {
  private static final String TRAN_TYPE = "PURCHASE";

  private final CardLimitRepository cardLimitRepository;
  private final VelocityCounterRepository velocityCounterRepository;

  public DailyCountRule(
      CardLimitRepository cardLimitRepository, VelocityCounterRepository velocityCounterRepository) {
    this.cardLimitRepository = cardLimitRepository;
    this.velocityCounterRepository = velocityCounterRepository;
  }

  /** Reflective config-driven registration factory; see {@code CheckLimits#setConfiguration}. */
  public static DailyCountRule create(
      CardLimitRepository cardLimitRepository, VelocityCounterRepository velocityCounterRepository) {
    return new DailyCountRule(cardLimitRepository, velocityCounterRepository);
  }

  @Override
  public Optional<String> evaluate(long cardId, long amountMinorUnits) {
    for (CardLimit limit : cardLimitRepository.findApplicableLimits(cardId, TRAN_TYPE)) {
      if ("DAILY".equals(limit.period()) && limit.maxCount() != null) {
        int countSoFar =
            velocityCounterRepository
                .findDaily(cardId, TRAN_TYPE, LocalDate.now())
                .map(VelocityCounterRow::txnCount)
                .orElse(0);
        if (countSoFar >= limit.maxCount()) {
          return Optional.of("65");
        }
      }
    }
    return Optional.empty();
  }
}
