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

  private static Context ctxFor(Card card, java.time.LocalDate businessDate) {
    var repo = Mockito.mock(CardRepository.class);
    when(repo.findByPanHash(any())).thenReturn(java.util.Optional.of(card));
    Context ctx = new Context();
    ctx.put(TxnContextKeys.PAN_HASH, new byte[] {1, 2, 3});
    ctx.put(TxnContextKeys.BUSINESS_DATE, businessDate);
    new CheckCard(repo).prepare(1L, ctx);
    return ctx;
  }

  private static Card card(String status, String expiryYymm) {
    return new Card(1L, 1L, "970436", "5540", expiryYymm, status, "crd_test0004", "Test Holder");
  }

  @Test
  @org.junit.jupiter.api.DisplayName("CARDS-G18: a PIN_BLOCKED card is declined RC 75 (docs/03 §8)")
  void should_declineRc75_when_cardIsPinBlocked() {
    Context ctx = ctxFor(card("PIN_BLOCKED", "3006"), java.time.LocalDate.of(2026, 9, 22));

    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("75");
  }

  @Test
  @org.junit.jupiter.api.DisplayName("CARDS-G18: LOST and STOLEN are declined RC 62")
  void should_declineRc62_when_cardIsLostOrStolen() {
    for (String status : java.util.List.of("LOST", "STOLEN")) {
      Context ctx = ctxFor(card(status, "3006"), java.time.LocalDate.of(2026, 9, 22));
      assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).as(status).isEqualTo("62");
    }
  }

  @Test
  @org.junit.jupiter.api.DisplayName(
      "CARDS-G18: expiry flips to RC 54 exactly at the month boundary, as the Admin API reads it")
  void should_declineRc54_fromTheFirstDayAfterTheExpiryMonth() {
    Context lastValidDay = ctxFor(card("ACTIVE", "2608"), java.time.LocalDate.of(2026, 8, 31));
    Context firstExpiredDay = ctxFor(card("ACTIVE", "2608"), java.time.LocalDate.of(2026, 9, 1));

    assertThat(lastValidDay.<String>get(TxnContextKeys.RESPONSE_CODE)).isNull();
    assertThat(firstExpiredDay.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("54");
  }

  @Test
  @org.junit.jupiter.api.DisplayName(
      "CARDS-G18: a BLOCKED card past expiry reads EXPIRED in the Admin API, so it declines RC 54")
  void should_declineRc54_when_blockedCardIsPastExpiry() {
    Context ctx = ctxFor(card("BLOCKED", "2001"), java.time.LocalDate.of(2026, 9, 22));

    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("54");
  }
}
