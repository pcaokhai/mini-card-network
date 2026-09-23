package io.mcn.issuer.adapter.txn.velocity;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.CardLimit;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import java.util.List;
import org.junit.jupiter.api.Test;

class PerTransactionAmountRuleTest {
  @Test
  void should_decline_rc61_when_amount_exceeds_per_txn_limit__MCN_803_AC1() {
    CardLimitRepository cardLimitRepository = mock(CardLimitRepository.class);
    when(cardLimitRepository.findApplicableLimits(1L, "PURCHASE"))
        .thenReturn(List.of(new CardLimit("PURCHASE", "PER_TXN", 10_000L, null)));
    PerTransactionAmountRule rule = new PerTransactionAmountRule(cardLimitRepository);

    assertThat(rule.evaluate(1L, 15_000L)).contains("61");
    assertThat(rule.evaluate(1L, 5_000L)).isEmpty();
  }
}
