package io.mcn.issuer.domain;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.LocalDate;
import org.junit.jupiter.api.Test;

class CardLifecycleTest {
  private static final LocalDate SEP_25_2026 = LocalDate.of(2026, 9, 25);

  @Test
  void should_notBeExpired_when_stillInsideTheExpiryMonth() {
    assertThat(CardLifecycle.isExpired("2609", SEP_25_2026)).isFalse();
  }

  @Test
  void should_beExpired_when_pastTheExpiryMonth() {
    assertThat(CardLifecycle.isExpired("2608", SEP_25_2026)).isTrue();
  }

  @Test
  void should_readExpired_when_activeOrBlockedCardIsPastExpiry() {
    assertThat(CardLifecycle.effectiveStatus("ACTIVE", "2608", SEP_25_2026)).isEqualTo("EXPIRED");
    assertThat(CardLifecycle.effectiveStatus("BLOCKED", "2608", SEP_25_2026)).isEqualTo("EXPIRED");
  }

  @Test
  void should_keepTheStoredStatus_when_notExpiredOrAStrongerStatus() {
    assertThat(CardLifecycle.effectiveStatus("ACTIVE", "2811", SEP_25_2026)).isEqualTo("ACTIVE");
    assertThat(CardLifecycle.effectiveStatus("LOST", "2608", SEP_25_2026)).isEqualTo("LOST");
    assertThat(CardLifecycle.effectiveStatus("STOLEN", "2608", SEP_25_2026)).isEqualTo("STOLEN");
    assertThat(CardLifecycle.effectiveStatus("PIN_BLOCKED", "2608", SEP_25_2026))
        .isEqualTo("PIN_BLOCKED");
  }
}
