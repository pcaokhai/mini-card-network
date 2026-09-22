package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.ReversalWithoutOriginalRepository;
import io.mcn.issuer.adapter.persistence.TranLogRepository;
import io.mcn.issuer.adapter.persistence.TranLogRow;
import java.time.LocalDate;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

class DeduplicateTest {

  @Test
  void firstRequestIsPrepared() {
    var repo = Mockito.mock(TranLogRepository.class);
    when(repo.findByDedupeKey(any(), any(), any(), any(), any(), any()))
        .thenReturn(java.util.Optional.empty());
    Context ctx = freshContext();

    int result = new Deduplicate(repo).prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
    assertThat(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE)).isFalse();
  }

  @Test
  void repeatRequestIsMarkedDuplicateNotAborted() {
    var repo = Mockito.mock(TranLogRepository.class);
    var stored =
        new TranLogRow(
            LocalDate.now(),
            "0200",
            "PURCHASE",
            "000000",
            "970499",
            "00000042",
            "GOCPHO000000001",
            "000001",
            "0922120000",
            "12345678",
            10000L,
            "704",
            null,
            "APPROVED",
            "00",
            "A1B2C3",
            null);
    when(repo.findByDedupeKey(any(), any(), any(), any(), any(), any()))
        .thenReturn(java.util.Optional.of(stored));
    Context ctx = freshContext();

    // Duplicates are NOT aborted - they flow through to Respond, which replays the stored
    // outcome.
    int result = new Deduplicate(repo).prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
    assertThat(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE)).isTrue();
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("00");
  }

  @Test
  void prepare_laterOriginalForAKnownReversalWithoutOriginal_isDeclinedRC94_MCN_402_AC2() {
    var tranLog = Mockito.mock(TranLogRepository.class);
    var rwo = Mockito.mock(ReversalWithoutOriginalRepository.class);
    when(rwo.findByKey(eq("0200"), eq("000001"), eq("0922120000"), eq("970499     ")))
        .thenReturn(true);
    Context ctx = freshContext();

    int result = new Deduplicate(tranLog, rwo).prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("94");
    org.mockito.Mockito.verifyNoInteractions(tranLog);
  }

  private Context freshContext() {
    ISOMsg msg = new ISOMsg("0200");
    msg.set(7, "0922120000");
    msg.set(11, "000001");
    msg.set(41, "GOCPHO00");
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, msg);
    ctx.put(TxnContextKeys.ACQUIRER_ID, "970499");
    ctx.put(TxnContextKeys.PROCESSING_CODE, "000000");
    return ctx;
  }
}
