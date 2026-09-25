package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.AccountLockRepository;
import io.mcn.issuer.adapter.persistence.AccountRow;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.VelocityCounterRepository;
import java.sql.Connection;
import java.sql.SQLException;
import javax.sql.DataSource;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

class AuthorizeTest {

  private final TranLogRepository tranLog = Mockito.mock(TranLogRepository.class);

  private static DataSource fakeDataSource() throws SQLException {
    DataSource ds = Mockito.mock(DataSource.class);
    Connection conn = Mockito.mock(Connection.class);
    when(ds.getConnection()).thenReturn(conn);
    return ds;
  }

  @Test
  void sufficientFundsApprovesAndPostsLedgerAndIncrementsVelocity() throws Exception {
    var lockRepo = Mockito.mock(AccountLockRepository.class);
    var ledgerRepo = Mockito.mock(LedgerRepository.class);
    var velocityCounterRepo = Mockito.mock(VelocityCounterRepository.class);
    when(lockRepo.lockAndGet(any(), anyLong()))
        .thenReturn(new AccountRow(1L, 100_000L, 100_000L, 0L, 0L));

    Context ctx = new Context();
    ctx.put(TxnContextKeys.ACCOUNT_ID, 1L);
    ctx.put(TxnContextKeys.CARD_ID, 7L);
    ctx.put(TxnContextKeys.AMOUNT, 10_000L);
    ctx.put(TxnContextKeys.TRAN_ID, 42L);

    var authorize =
        new Authorize(
            lockRepo,
            ledgerRepo,
            velocityCounterRepo,
            new AuthCodeGenerator(),
            tranLog,
            fakeDataSource());
    int result = authorize.prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("00");
    assertThat(ctx.<String>get(TxnContextKeys.AUTH_CODE)).hasSize(6);
    Mockito.verify(lockRepo).adjust(any(), Mockito.eq(1L), Mockito.eq(-10_000L));
    // S1: the APPROVED outcome is written in Authorize's own transaction, with the money
    Mockito.verify(tranLog)
        .updateOutcome(
            any(),
            Mockito.eq(42L),
            any(),
            Mockito.eq("APPROVED"),
            Mockito.eq("00"),
            Mockito.eq(ctx.<String>get(TxnContextKeys.AUTH_CODE)),
            Mockito.isNull(),
            Mockito.isNull());
    Mockito.verify(ledgerRepo)
        .postPurchase(any(), Mockito.eq(42L), any(), Mockito.eq(1L), Mockito.eq(10_000L), any());
    Mockito.verify(velocityCounterRepo)
        .incrementDaily(any(), Mockito.eq(7L), Mockito.eq("PURCHASE"), any(), Mockito.eq(10_000L));
  }

  @Test
  void insufficientFundsDeclinesRc51WithoutPostingLedgerOrIncrementingVelocity() throws Exception {
    var lockRepo = Mockito.mock(AccountLockRepository.class);
    var ledgerRepo = Mockito.mock(LedgerRepository.class);
    var velocityCounterRepo = Mockito.mock(VelocityCounterRepository.class);
    when(lockRepo.lockAndGet(any(), anyLong()))
        .thenReturn(new AccountRow(1L, 5_000L, 5_000L, 0L, 0L));

    Context ctx = new Context();
    ctx.put(TxnContextKeys.ACCOUNT_ID, 1L);
    ctx.put(TxnContextKeys.CARD_ID, 7L);
    ctx.put(TxnContextKeys.AMOUNT, 10_000L);

    var authorize =
        new Authorize(
            lockRepo,
            ledgerRepo,
            velocityCounterRepo,
            new AuthCodeGenerator(),
            tranLog,
            fakeDataSource());
    authorize.prepare(1L, ctx);

    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("51");
    Mockito.verifyNoInteractions(ledgerRepo, velocityCounterRepo);
  }

  @Test
  void priorDeclineFromAnEarlierParticipantIsPreservedUnchanged() throws Exception {
    var lockRepo = Mockito.mock(AccountLockRepository.class);
    var ledgerRepo = Mockito.mock(LedgerRepository.class);
    var velocityCounterRepo = Mockito.mock(VelocityCounterRepository.class);
    Context ctx = new Context();
    ctx.put(TxnContextKeys.RESPONSE_CODE, "61");

    var authorize =
        new Authorize(
            lockRepo,
            ledgerRepo,
            velocityCounterRepo,
            new AuthCodeGenerator(),
            tranLog,
            fakeDataSource());
    authorize.prepare(1L, ctx);

    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("61");
    Mockito.verifyNoInteractions(lockRepo, ledgerRepo, velocityCounterRepo);
  }

  @Test
  @org.junit.jupiter.api.DisplayName(
      "R-1: the overdraft floor is enforced in the debit path, under the account lock")
  void debitIsApprovedDownToTheOverdraftFloorAndDeclinedPastIt() throws Exception {
    var lockRepo = Mockito.mock(AccountLockRepository.class);
    // available 5 000 with a 10 000 overdraft: the floor is -10 000
    when(lockRepo.lockAndGet(any(), anyLong()))
        .thenReturn(new AccountRow(1L, 5_000L, 5_000L, 10_000L, 0L));
    var authorize =
        new Authorize(
            lockRepo,
            Mockito.mock(LedgerRepository.class),
            Mockito.mock(VelocityCounterRepository.class),
            new AuthCodeGenerator(),
            tranLog,
            fakeDataSource());

    Context toTheFloor = new Context();
    toTheFloor.put(TxnContextKeys.ACCOUNT_ID, 1L);
    toTheFloor.put(TxnContextKeys.AMOUNT, 15_000L);
    toTheFloor.put(TxnContextKeys.TRAN_ID, 43L);
    authorize.prepare(1L, toTheFloor);
    Context pastTheFloor = new Context();
    pastTheFloor.put(TxnContextKeys.ACCOUNT_ID, 1L);
    pastTheFloor.put(TxnContextKeys.AMOUNT, 15_001L);
    pastTheFloor.put(TxnContextKeys.TRAN_ID, 44L);
    authorize.prepare(2L, pastTheFloor);

    assertThat(toTheFloor.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("00");
    assertThat(pastTheFloor.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("51");
  }
}
