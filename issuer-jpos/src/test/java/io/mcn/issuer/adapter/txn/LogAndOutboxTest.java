package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.verify;

import io.mcn.issuer.adapter.persistence.TranLogRepository;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.mockito.Mockito;

class LogAndOutboxTest {

  @Test
  void everyOutcomeWritesATranLogRow() throws Exception {
    var repo = Mockito.mock(TranLogRepository.class);
    ISOMsg request = new ISOMsg("0200");
    request.set(3, "000000");
    request.set(4, "000000010000");
    request.set(7, "0922120000");
    request.set(11, "000001");
    request.set(32, "970499");
    request.set(37, "GOCPHO000000");
    request.set(41, "GOCPHO00");
    request.set(42, "MERCHANT123456");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.RESPONSE_CODE, "14");
    ctx.put(TxnContextKeys.ACQUIRER_ID, "970499");
    ctx.put(TxnContextKeys.AMOUNT, 10000L);
    ctx.put(TxnContextKeys.IS_DUPLICATE, false);

    new LogAndOutbox(repo).commit(1L, ctx);

    var captor = ArgumentCaptor.forClass(io.mcn.issuer.adapter.persistence.TranLogRow.class);
    verify(repo).insert(captor.capture());
    assertThat(captor.getValue().responseCode()).isEqualTo("14");
  }

  @Test
  void duplicateOutcomeIsNotLoggedAgain() throws Exception {
    var repo = Mockito.mock(TranLogRepository.class);
    ISOMsg request = new ISOMsg("0200");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.IS_DUPLICATE, true);

    new LogAndOutbox(repo).commit(1L, ctx);

    Mockito.verifyNoInteractions(repo);
  }
}
