package io.mcn.issuer.adapter.txn;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.Card;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.OriginalTransactionRow;
import io.mcn.issuer.adapter.persistence.ReversalWithoutOriginalRepository;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import java.time.LocalDate;
import java.util.Optional;
import javax.sql.DataSource;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class LocateAndReverseTest {

  @Test
  void prepare_foundOriginal_postsReversalAndMarksReversed_MCN_402_AC1() throws Exception {
    var tranLog = mock(TranLogRepository.class);
    var ledger = mock(LedgerRepository.class);
    when(tranLog.findByReversalKey("0200", "000123", "0922140000", "970499     "))
        .thenReturn(Optional.of(originalRow(42L, "APPROVED", 10_000L)));
    when(tranLog.markReversed(any(), eq(42L), any())).thenReturn(true);
    Context ctx = contextWithReversalKey();

    newLocateAndReverse(tranLog, ledger).prepare(0, ctx);

    verify(tranLog).markReversed(any(), eq(42L), any());
    verify(ledger).postReversal(any(), eq(42L), any(), anyLong(), eq(10_000L), eq("704"));
  }

  /** A 0420 and its 0421 repeat in flight together: only the one that flips APPROVED posts. */
  @Test
  void prepare_concurrentReversalAlreadyFlippedTheRow_postsNoJournal__MCN_401() throws Exception {
    var tranLog = mock(TranLogRepository.class);
    var ledger = mock(LedgerRepository.class);
    when(tranLog.findByReversalKey(any(), any(), any(), any()))
        .thenReturn(Optional.of(originalRow(42L, "APPROVED", 10_000L)));
    when(tranLog.markReversed(any(), eq(42L), any())).thenReturn(false);

    newLocateAndReverse(tranLog, ledger).prepare(0, contextWithReversalKey());

    verifyNoInteractions(ledger);
  }

  /**
   * The 0200 is still being authorised: answering "00" now would let it approve and never be
   * reversed, so the reversal fails and the acquirer's SAF repeats it.
   */
  @Test
  void prepare_originalStillInFlight_failsSoTheAcquirerRepeats__MCN_401() {
    var tranLog = mock(TranLogRepository.class);
    var ledger = mock(LedgerRepository.class);
    when(tranLog.findByReversalKey(any(), any(), any(), any()))
        .thenReturn(Optional.of(originalRow(42L, "RECEIVED", 10_000L)));

    org.assertj.core.api.Assertions.assertThatThrownBy(
            () -> newLocateAndReverse(tranLog, ledger).prepare(0, contextWithReversalKey()))
        .isInstanceOf(IllegalStateException.class);
    verifyNoInteractions(ledger);
  }

  private static LocateAndReverse newLocateAndReverse(
      TranLogRepository tranLog, LedgerRepository ledger) throws Exception {
    var cards = mock(CardRepository.class);
    when(cards.findById(7L)).thenReturn(Optional.of(card(7L, 99L)));
    var ds = mock(DataSource.class);
    when(ds.getConnection()).thenReturn(mock(java.sql.Connection.class));
    return new LocateAndReverse(
        tranLog, ledger, mock(ReversalWithoutOriginalRepository.class), cards, ds);
  }

  @Test
  void prepare_originalNotFound_recordsReversalWithoutOriginal_MCN_402_AC2() {
    var tranLog = mock(TranLogRepository.class);
    when(tranLog.findByReversalKey(any(), any(), any(), any())).thenReturn(Optional.empty());
    var rwo = mock(ReversalWithoutOriginalRepository.class);
    Context ctx = contextWithReversalKey();

    new LocateAndReverse(tranLog, mock(LedgerRepository.class), rwo, mock(DataSource.class))
        .prepare(0, ctx);

    verify(rwo).insert(eq("0200"), eq("000123"), eq("0922140000"), eq("970499     "), any());
  }

  @Test
  void prepare_alreadyReversedOriginal_isIdempotentNoLedgerEffect_MCN_402_AC1() {
    var tranLog = mock(TranLogRepository.class);
    var ledger = mock(LedgerRepository.class);
    when(tranLog.findByReversalKey(any(), any(), any(), any()))
        .thenReturn(Optional.of(originalRow(42L, "REVERSED", 10_000L)));
    Context ctx = contextWithReversalKey();

    new LocateAndReverse(
            tranLog, ledger, mock(ReversalWithoutOriginalRepository.class), mock(DataSource.class))
        .prepare(0, ctx);

    verifyNoInteractions(ledger); // idempotent repeat: no second journal entry
  }

  private static Context contextWithReversalKey() {
    Context ctx = new Context();
    ctx.put(TxnContextKeys.ORIGINAL_MTI, "0200");
    ctx.put(TxnContextKeys.ORIGINAL_STAN, "000123");
    ctx.put(TxnContextKeys.ORIGINAL_DE7, "0922140000");
    ctx.put(TxnContextKeys.ORIGINAL_ACQUIRER, "970499     ");
    ctx.put(TxnContextKeys.BUSINESS_DATE, LocalDate.now());
    return ctx;
  }

  private static OriginalTransactionRow originalRow(long tranId, String status, long amount) {
    return new OriginalTransactionRow(tranId, LocalDate.now(), 7L, amount, "704", status);
  }

  private static Card card(long cardId, long accountId) {
    return new Card(cardId, accountId, "970436", "0001", "3012", "ACTIVE", "crd_x", "Test");
  }
}
