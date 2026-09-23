package io.mcn.issuer.adapter.txn.velocity;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.CardLimit;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRow;
import java.time.LocalDate;
import java.util.List;
import java.util.Optional;
import org.junit.jupiter.api.Test;

class DailyCountRuleTest {
  @Test
  void should_decline_rc65_when_todays_count_at_or_above_daily_limit__MCN_803_AC1() {
    CardLimitRepository cardLimitRepository = mock(CardLimitRepository.class);
    VelocityCounterRepository velocityCounterRepository = mock(VelocityCounterRepository.class);
    when(cardLimitRepository.findApplicableLimits(1L, "PURCHASE"))
        .thenReturn(List.of(new CardLimit("PURCHASE", "DAILY", null, 5)));
    when(velocityCounterRepository.findDaily(1L, "PURCHASE", LocalDate.now()))
        .thenReturn(
            Optional.of(
                new VelocityCounterRow(
                    1L, "PURCHASE", "DAILY", LocalDate.now().toString(), 5, 0L)));

    DailyCountRule rule = new DailyCountRule(cardLimitRepository, velocityCounterRepository);

    assertThat(rule.evaluate(1L, 1_000L)).contains("65");
  }

  @Test
  void should_allow_when_todays_count_below_daily_limit__MCN_803_AC1() {
    CardLimitRepository cardLimitRepository = mock(CardLimitRepository.class);
    VelocityCounterRepository velocityCounterRepository = mock(VelocityCounterRepository.class);
    when(cardLimitRepository.findApplicableLimits(1L, "PURCHASE"))
        .thenReturn(List.of(new CardLimit("PURCHASE", "DAILY", null, 5)));
    when(velocityCounterRepository.findDaily(1L, "PURCHASE", LocalDate.now()))
        .thenReturn(Optional.empty());

    DailyCountRule rule = new DailyCountRule(cardLimitRepository, velocityCounterRepository);

    assertThat(rule.evaluate(1L, 1_000L)).isEmpty();
  }
}
