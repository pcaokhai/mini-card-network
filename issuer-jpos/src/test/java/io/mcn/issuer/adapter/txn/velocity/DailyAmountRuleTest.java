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

class DailyAmountRuleTest {
  @Test
  void should_decline_rc61_when_todays_amount_plus_this_txn_exceeds_daily_limit__MCN_803_AC1() {
    CardLimitRepository cardLimitRepository = mock(CardLimitRepository.class);
    VelocityCounterRepository velocityCounterRepository = mock(VelocityCounterRepository.class);
    when(cardLimitRepository.findApplicableLimits(1L, "PURCHASE"))
        .thenReturn(List.of(new CardLimit("PURCHASE", "DAILY", 50_000L, null)));
    when(velocityCounterRepository.findDaily(1L, "PURCHASE", LocalDate.now()))
        .thenReturn(
            Optional.of(
                new VelocityCounterRow(1L, "PURCHASE", "DAILY", LocalDate.now().toString(), 3, 45_000L)));

    DailyAmountRule rule = new DailyAmountRule(cardLimitRepository, velocityCounterRepository);

    assertThat(rule.evaluate(1L, 10_000L)).contains("61"); // 45,000 + 10,000 > 50,000
    assertThat(rule.evaluate(1L, 4_000L)).isEmpty(); // 45,000 + 4,000 <= 50,000
  }
}
