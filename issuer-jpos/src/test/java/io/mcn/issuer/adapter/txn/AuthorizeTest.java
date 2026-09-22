package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.AccountLockRepository;
import io.mcn.issuer.adapter.persistence.AccountRow;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import java.sql.Connection;
import java.sql.SQLException;
import javax.sql.DataSource;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

class AuthorizeTest {

  private static DataSource fakeDataSource() throws SQLException {
    DataSource ds = Mockito.mock(DataSource.class);
    Connection conn = Mockito.mock(Connection.class);
    when(ds.getConnection()).thenReturn(conn);
    return ds;
  }

  @Test
  void sufficientFundsApprovesAndPostsLedger() throws Exception {
    var lockRepo = Mockito.mock(AccountLockRepository.class);
    var ledgerRepo = Mockito.mock(LedgerRepository.class);
    when(lockRepo.lockAndGet(any(), anyLong()))
        .thenReturn(new AccountRow(1L, 100_000L, 100_000L, 0L, 0L));
    when(lockRepo.debit(any(), anyLong(), anyLong(), anyLong())).thenReturn(true);

    Context ctx = new Context();
    ctx.put(TxnContextKeys.ACCOUNT_ID, 1L);
    ctx.put(TxnContextKeys.AMOUNT, 10_000L);
    ctx.put(TxnContextKeys.TRAN_ID, 42L);

    var authorize = new Authorize(lockRepo, ledgerRepo, new AuthCodeGenerator(), fakeDataSource());
    int result = authorize.prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("00");
    assertThat(ctx.<String>get(TxnContextKeys.AUTH_CODE)).hasSize(6);
    Mockito.verify(ledgerRepo)
        .postPurchase(any(), Mockito.eq(42L), any(), Mockito.eq(1L), Mockito.eq(10_000L), any());
  }

  @Test
  void insufficientFundsDeclinesRc51WithoutPostingLedger() throws Exception {
    var lockRepo = Mockito.mock(AccountLockRepository.class);
    var ledgerRepo = Mockito.mock(LedgerRepository.class);
    when(lockRepo.lockAndGet(any(), anyLong()))
        .thenReturn(new AccountRow(1L, 5_000L, 5_000L, 0L, 0L));

    Context ctx = new Context();
    ctx.put(TxnContextKeys.ACCOUNT_ID, 1L);
    ctx.put(TxnContextKeys.AMOUNT, 10_000L);

    var authorize = new Authorize(lockRepo, ledgerRepo, new AuthCodeGenerator(), fakeDataSource());
    authorize.prepare(1L, ctx);

    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("51");
    Mockito.verifyNoInteractions(ledgerRepo);
  }

  @Test
  void priorDeclineFromAnEarlierParticipantIsPreservedUnchanged() throws Exception {
    var lockRepo = Mockito.mock(AccountLockRepository.class);
    var ledgerRepo = Mockito.mock(LedgerRepository.class);
    Context ctx = new Context();
    ctx.put(TxnContextKeys.RESPONSE_CODE, "61");

    var authorize = new Authorize(lockRepo, ledgerRepo, new AuthCodeGenerator(), fakeDataSource());
    authorize.prepare(1L, ctx);

    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("61");
    Mockito.verifyNoInteractions(lockRepo, ledgerRepo);
  }
}
