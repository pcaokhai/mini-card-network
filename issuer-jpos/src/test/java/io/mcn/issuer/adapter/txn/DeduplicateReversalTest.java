package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.TranLogRow;
import java.time.LocalDate;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

class DeduplicateReversalTest {

  @Test
  void prepare_firstReversalIsNotDuplicate_MCN_402_AC3() {
    var repo = Mockito.mock(TranLogRepository.class);
    when(repo.findByDedupeKey(any(), any(), any(), any(), any(), any()))
        .thenReturn(java.util.Optional.empty());
    Context ctx = contextWithReversalRequest();

    int result = new DeduplicateReversal(repo).prepare(0, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
    assertThat(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE)).isFalse();
  }

  @Test
  void prepare_exactResendOfSameReversalIsMarkedDuplicate_MCN_402_AC3() {
    var repo = Mockito.mock(TranLogRepository.class);
    when(repo.findByDedupeKey(any(), any(), any(), any(), eq("0420"), any()))
        .thenReturn(java.util.Optional.of(storedReversalRow()));
    Context ctx = contextWithReversalRequest();

    new DeduplicateReversal(repo).prepare(0, ctx);

    assertThat(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE)).isTrue();
  }

  private static Context contextWithReversalRequest() {
    ISOMsg msg = new ISOMsg("0420");
    msg.set(7, "0922140055");
    msg.set(11, "000456");
    msg.set(41, "GOCPHO00");
    msg.set(90, "0200" + "000123" + "0922140000" + "970499     " + "00000000000");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, msg);
    ctx.put(TxnContextKeys.ACQUIRER_ID, "970499");
    ctx.put(TxnContextKeys.BUSINESS_DATE, LocalDate.now());
    return ctx;
  }

  private static TranLogRow storedReversalRow() {
    return new TranLogRow(
        LocalDate.now(),
        "0420",
        "REVERSAL",
        "000000",
        "970499",
        "GOCPHO00",
        "GOCPHO000000001",
        "000456",
        "0922140055",
        "RRN000000456",
        10000L,
        "704",
        null,
        "RECEIVED",
        null,
        null,
        null);
  }
}
