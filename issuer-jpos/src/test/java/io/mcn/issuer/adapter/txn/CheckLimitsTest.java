package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.ABORTED;
import static org.jpos.transaction.TransactionConstants.PREPARED;

import io.mcn.issuer.application.VelocityRule;
import java.util.List;
import java.util.Optional;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class CheckLimitsTest {

  @Test
  void should_decline_using_whichever_rule_in_the_configured_list_first_declines__MCN_803_AC2() {
    VelocityRule alwaysAllow = (cardId, amount) -> Optional.empty();
    VelocityRule declinesRc65 = (cardId, amount) -> Optional.of("65");
    CheckLimits participant = new CheckLimits(List.of(alwaysAllow, declinesRc65));

    Context ctx = new Context();
    ctx.put(TxnContextKeys.CARD_ID, 1L);
    ctx.put(TxnContextKeys.AMOUNT, 1_000L);

    int result = participant.prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("65");
  }

  @Test
  void should_prepare_when_every_configured_rule_allows__MCN_803_AC2() {
    VelocityRule alwaysAllow = (cardId, amount) -> Optional.empty();
    CheckLimits participant = new CheckLimits(List.of(alwaysAllow, alwaysAllow));

    Context ctx = new Context();
    ctx.put(TxnContextKeys.CARD_ID, 1L);
    ctx.put(TxnContextKeys.AMOUNT, 1_000L);

    assertThat(participant.prepare(1L, ctx)).isEqualTo(PREPARED);
  }

  @Test
  void should_add_a_fourth_rule_with_zero_changes_to_checklimits_body__MCN_803_AC2() {
    // Proves the AC literally: a brand-new, throwaway rule type is constructed and passed in
    // via the existing constructor - no CheckLimits.java source edit was needed to support it.
    VelocityRule throwawayFourthRule =
        (cardId, amount) -> amount > 999_999L ? Optional.of("61") : Optional.empty();
    CheckLimits participant = new CheckLimits(List.of(throwawayFourthRule));

    Context ctx = new Context();
    ctx.put(TxnContextKeys.CARD_ID, 1L);
    ctx.put(TxnContextKeys.AMOUNT, 1_000_000L);

    assertThat(participant.prepare(1L, ctx)).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("61");
  }
}
