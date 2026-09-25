package io.mcn.issuer.adapter.txn;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.AccountLockRepository;
import io.mcn.issuer.adapter.persistence.AccountRow;
import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.OriginalTransactionRow;
import io.mcn.issuer.adapter.persistence.ReversalWithoutOriginalRepository;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import java.time.LocalDate;
import java.util.Map;
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
    when(ledger.postReversalOf(any(), eq(42L), any())).thenReturn(Map.of(99L, 10_000L));
    var accounts = mock(AccountLockRepository.class);
    when(accounts.adjust(any(), eq(99L), eq(10_000L)))
        .thenReturn(new AccountRow(99L, 10_000L, 10_000L, 0L, 1L));
    var velocity = mock(VelocityCounterRepository.class);
    Context ctx = contextWithReversalKey();

    newLocateAndReverse(tranLog, ledger, accounts, velocity).prepare(0, ctx);

    verify(tranLog).markReversed(any(), eq(42L), any());
    verify(ledger).postReversalOf(any(), eq(42L), any());
    verify(accounts).adjust(any(), eq(99L), eq(10_000L)); // balance moves with its journal
    // R-3: the reversed debit no longer counts against the day's velocity limits
    verify(velocity).decrementDaily(any(), eq(7L), eq("PURCHASE"), any(), eq(10_000L));
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
    return newLocateAndReverse(
        tranLog, ledger, mock(AccountLockRepository.class), mock(VelocityCounterRepository.class));
  }

  private static LocateAndReverse newLocateAndReverse(
      TranLogRepository tranLog,
      LedgerRepository ledger,
      AccountLockRepository accounts,
      VelocityCounterRepository velocity)
      throws Exception {
    var ds = mock(DataSource.class);
    when(ds.getConnection()).thenReturn(mock(java.sql.Connection.class));
    return new LocateAndReverse(
        tranLog,
        ledger,
        mock(ReversalWithoutOriginalRepository.class),
        accounts,
        velocity,
        mock(AuditLogRepository.class),
        ds);
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
}
