package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.ABORTED;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.Card;
import io.mcn.issuer.adapter.persistence.CardRepository;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

class CheckCardTest {

  @Test
  void unknownCardAbortsWithRc14() {
    var repo = Mockito.mock(CardRepository.class);
    when(repo.findByPanHash(any())).thenReturn(java.util.Optional.empty());
    Context ctx = new Context();
    ctx.put(TxnContextKeys.PAN_HASH, new byte[] {1, 2, 3});

    int result = new CheckCard(repo).prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("14");
  }

  @Test
  void blockedCardAbortsWithRc62() {
    var repo = Mockito.mock(CardRepository.class);
    when(repo.findByPanHash(any()))
        .thenReturn(
            java.util.Optional.of(
                new Card(
                    1L, 1L, "970436", "3310", "2707", "BLOCKED", "crd_test0001", "Test Holder")));
    Context ctx = new Context();
    ctx.put(TxnContextKeys.PAN_HASH, new byte[] {1, 2, 3});
    ctx.put(TxnContextKeys.BUSINESS_DATE, java.time.LocalDate.of(2026, 9, 22));

    int result = new CheckCard(repo).prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("62");
  }

  @Test
  void expiredCardAbortsWithRc54() {
    var repo = Mockito.mock(CardRepository.class);
    when(repo.findByPanHash(any()))
        .thenReturn(
            java.util.Optional.of(
                new Card(
                    1L, 1L, "970436", "7765", "2001", "ACTIVE", "crd_test0002", "Test Holder")));
    Context ctx = new Context();
    ctx.put(TxnContextKeys.PAN_HASH, new byte[] {1, 2, 3});
    ctx.put(TxnContextKeys.BUSINESS_DATE, java.time.LocalDate.of(2026, 9, 22));

    int result = new CheckCard(repo).prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("54");
  }

  @Test
  void activeUnexpiredCardIsPrepared() {
    var repo = Mockito.mock(CardRepository.class);
    when(repo.findByPanHash(any()))
        .thenReturn(
            java.util.Optional.of(
                new Card(
                    1L, 1L, "970436", "4417", "2811", "ACTIVE", "crd_test0003", "Test Holder")));
    Context ctx = new Context();
    ctx.put(TxnContextKeys.PAN_HASH, new byte[] {1, 2, 3});
    ctx.put(TxnContextKeys.BUSINESS_DATE, java.time.LocalDate.of(2026, 9, 22));

    int result = new CheckCard(repo).prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
    assertThat(ctx.<Long>get(TxnContextKeys.ACCOUNT_ID)).isEqualTo(1L);
  }
}
