package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.ABORTED;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.CardLimit;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

class CheckLimitsTest {

  @Test
  void amountOverPerTxnLimitAbortsWithRc61() {
    var repo = Mockito.mock(CardLimitRepository.class);
    when(repo.findApplicableLimits(anyLong(), any()))
        .thenReturn(java.util.List.of(new CardLimit("PURCHASE", "PER_TXN", 5_000L, null)));
    Context ctx = new Context();
    ctx.put(TxnContextKeys.CARD_ID, 1L);
    ctx.put(TxnContextKeys.PROCESSING_CODE, "000000");
    ctx.put(TxnContextKeys.AMOUNT, 10_000L);

    int result = new CheckLimits(repo).prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("61");
  }

  @Test
  void countOverDailyLimitAbortsWithRc65() {
    var repo = Mockito.mock(CardLimitRepository.class);
    when(repo.findApplicableLimits(anyLong(), any()))
        .thenReturn(java.util.List.of(new CardLimit("PURCHASE", "DAILY", null, 3)));
    when(repo.countToday(anyLong(), any())).thenReturn(3);
    Context ctx = new Context();
    ctx.put(TxnContextKeys.CARD_ID, 1L);
    ctx.put(TxnContextKeys.PROCESSING_CODE, "000000");
    ctx.put(TxnContextKeys.AMOUNT, 1_000L);

    int result = new CheckLimits(repo).prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("65");
  }

  @Test
  void withinAllLimitsIsPrepared() {
    var repo = Mockito.mock(CardLimitRepository.class);
    when(repo.findApplicableLimits(anyLong(), any())).thenReturn(java.util.List.of());
    Context ctx = new Context();
    ctx.put(TxnContextKeys.CARD_ID, 1L);
    ctx.put(TxnContextKeys.PROCESSING_CODE, "000000");
    ctx.put(TxnContextKeys.AMOUNT, 1_000L);

    int result = new CheckLimits(repo).prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
  }
}
