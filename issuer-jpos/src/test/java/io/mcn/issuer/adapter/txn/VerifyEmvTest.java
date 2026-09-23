package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.ABORTED;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.crypto.ArqcSimulator;
import io.mcn.issuer.adapter.persistence.CardRepository;
import java.util.HexFormat;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;

class VerifyEmvTest {

  @Test
  void should_pass_through_when_no_de55_present__MCN_602_AC1() throws ISOException {
    VerifyEmv participant = new VerifyEmv(new ArqcSimulator(), mock(CardRepository.class));
    ISOMsg request = new ISOMsg("0200"); // no field 55 - manual/magstripe entry

    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);

    assertThat(participant.prepare(1L, ctx)).isEqualTo(PREPARED);
  }

  @Test
  void should_abort_rc30_on_malformed_tlv__MCN_602_AC1() throws ISOException {
    VerifyEmv participant = new VerifyEmv(new ArqcSimulator(), mock(CardRepository.class));
    ISOMsg request = new ISOMsg("0200");
    request.set(55, HexFormat.of().parseHex("9F3605")); // truncated length

    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.CARD_ID, 1L);
    ctx.put(TxnContextKeys.PAN, "9704360000000001");

    int result = participant.prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("30");
  }

  @Test
  void should_abort_rc05_on_non_increasing_atc__MCN_602_AC2() throws ISOException {
    CardRepository cardRepository = mock(CardRepository.class);
    when(cardRepository.updateEmvLastAtcIfIncreasing(anyLong(), anyInt())).thenReturn(false);

    VerifyEmv participant = new VerifyEmv(new ArqcSimulator(), cardRepository);
    ISOMsg request = new ISOMsg("0200");
    // 9F36 (ATC) len 02 value 0003
    request.set(55, HexFormat.of().parseHex("9F3602" + "0003"));

    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.CARD_ID, 1L);
    ctx.put(TxnContextKeys.PAN, "9704360000000001");

    int result = participant.prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat(ctx.<String>get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("05");
  }

  @Test
  void should_prepare_and_set_arpc_when_arqc_valid_and_atc_advances__MCN_602_AC2_AC3()
      throws ISOException {
    CardRepository cardRepository = mock(CardRepository.class);
    when(cardRepository.updateEmvLastAtcIfIncreasing(anyLong(), anyInt())).thenReturn(true);
    ArqcSimulator arqcSimulator = new ArqcSimulator();

    String pan = "9704360000000001";
    int atc = 3;
    byte[] un = new byte[] {0x01, 0x02, 0x03, 0x04};
    byte[] validArqc = arqcSimulator.computeArqcForTest(pan, atc, un);

    HexFormat hex = HexFormat.of();
    StringBuilder de55 = new StringBuilder();
    de55.append("9F3602").append(hex.formatHex(new byte[] {0x00, 0x03})); // ATC
    de55.append("9F26")
        .append(String.format("%02X", validArqc.length))
        .append(hex.formatHex(validArqc));
    de55.append("9F37").append(String.format("%02X", un.length)).append(hex.formatHex(un));

    VerifyEmv participant = new VerifyEmv(arqcSimulator, cardRepository);
    ISOMsg request = new ISOMsg("0200");
    request.set(55, hex.parseHex(de55.toString()));

    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.CARD_ID, 1L);
    ctx.put(TxnContextKeys.PAN, pan);

    int result = participant.prepare(1L, ctx);

    assertThat(result).isEqualTo(PREPARED);
    assertThat(ctx.<byte[]>get(TxnContextKeys.EMV_ARPC))
        .isEqualTo(arqcSimulator.computeArpc(validArqc));
  }
}
