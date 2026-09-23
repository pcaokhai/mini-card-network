package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;
import static org.jpos.transaction.TransactionConstants.ABORTED;
import static org.jpos.transaction.TransactionConstants.PREPARED;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.Mockito.when;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import io.mcn.issuer.adapter.crypto.PvvCalculator;
import io.mcn.issuer.adapter.persistence.CardRepository;
import java.io.File;
import java.util.HexFormat;
import java.util.Iterator;
import java.util.Map;
import javax.crypto.Cipher;
import javax.crypto.spec.SecretKeySpec;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.packager.GenericPackager;
import org.jpos.transaction.Context;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

class VerifySecurityTest {

  private static final String LMK_HEX =
      "00112233445566778899aabbccddeeff00112233445566778899aabbccddee";
  private static final byte[] ZAK = HexFormat.of().parseHex("3132333435363738393a3b3c3d3e3f40");
  private static final byte[] ZPK = HexFormat.of().parseHex("2132333435363738393a3b3c3d3e3f41");
  private static final String PAN = "9704360000004417";
  private static final long CARD_ID = 1L;

  private static GenericPackager packager() throws Exception {
    return new GenericPackager("src/dist/cfg/iso87ascii.xml");
  }

  @Test
  void should_abort_with_rc96_and_skip_pin_check_on_bad_mac__MCN_503_AC2() throws Exception {
    JCESecurityModule securityModule = new JCESecurityModule(LMK_HEX);
    CardRepository cardRepository = Mockito.mock(CardRepository.class);
    VerifySecurity participant = new VerifySecurity(securityModule, cardRepository, ZAK, ZPK);

    ISOMsg request = baseFields();
    request.set(
        52, HexFormat.of().formatHex(new byte[8])); // present but irrelevant - MAC fails first
    request.set(64, HexFormat.of().formatHex(new byte[8])); // deliberately wrong MAC
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.CARD_ID, CARD_ID);
    ctx.put(TxnContextKeys.PAN, PAN);

    int result = participant.prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat((String) ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("96");
    Mockito.verifyNoInteractions(cardRepository);
  }

  @Test
  void should_compute_identical_mac_to_gateway_go_on_shared_golden_vector__MCN_503_AC2()
      throws Exception {
    ObjectMapper mapper = new ObjectMapper();
    JsonNode vector =
        mapper.readTree(new File("../contracts/iso8583/vectors/mac/0200-with-mac.json"));
    byte[] vectorZak = HexFormat.of().parseHex(vector.get("zakHex").asText());
    byte[] expectedPacked = HexFormat.of().parseHex(vector.get("packedHexExcludingMac").asText());
    byte[] expectedMac = HexFormat.of().parseHex(vector.get("expectedMacHex").asText());

    ISOMsg msg = new ISOMsg();
    msg.setPackager(packager());
    msg.setMTI(vector.get("mti").asText());
    Iterator<Map.Entry<String, JsonNode>> it = vector.get("fields").fields();
    while (it.hasNext()) {
      Map.Entry<String, JsonNode> e = it.next();
      String key = e.getKey();
      if (key.equals("64")) continue; // DE 64 is genuinely absent from the MAC input
      msg.set(Integer.parseInt(key), e.getValue().asText());
    }
    byte[] packed = msg.pack();
    assertThat(packed).as("packer fidelity vs. gateway-go's Pack()").isEqualTo(expectedPacked);

    byte[] mac = new JCESecurityModule(LMK_HEX).computeMac(packed, vectorZak);

    assertThat(mac)
        .as("issuer MAC must be byte-identical to gateway-go's ComputeMAC")
        .isEqualTo(expectedMac);
  }

  @Test
  void should_decline_rc55_and_increment_pin_try_count_on_wrong_pvv__MCN_503_AC1()
      throws Exception {
    JCESecurityModule securityModule = new JCESecurityModule(LMK_HEX);
    CardRepository cardRepository = Mockito.mock(CardRepository.class);
    when(cardRepository.findPvv(CARD_ID))
        .thenReturn(java.util.Optional.of(PvvCalculator.computePvv(PAN, "1234")));
    when(cardRepository.incrementPinTryCount(CARD_ID)).thenReturn(1);
    VerifySecurity participant = new VerifySecurity(securityModule, cardRepository, ZAK, ZPK);

    ISOMsg request = goodMacRequest("0000", securityModule);
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.CARD_ID, CARD_ID);
    ctx.put(TxnContextKeys.PAN, PAN);

    int result = participant.prepare(1L, ctx);

    assertThat(result).isEqualTo(ABORTED);
    assertThat((String) ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("55");
    Mockito.verify(cardRepository).incrementPinTryCount(CARD_ID);
    Mockito.verify(cardRepository, Mockito.never()).blockForPin(anyLong());
  }

  @Test
  void should_block_card_and_return_rc75_on_third_wrong_pin__MCN_503_AC1() throws Exception {
    JCESecurityModule securityModule = new JCESecurityModule(LMK_HEX);
    CardRepository cardRepository = Mockito.mock(CardRepository.class);
    when(cardRepository.findPvv(CARD_ID))
        .thenReturn(java.util.Optional.of(PvvCalculator.computePvv(PAN, "1234")));
    when(cardRepository.incrementPinTryCount(CARD_ID))
        .thenReturn(3); // two prior failures + this one
    VerifySecurity participant = new VerifySecurity(securityModule, cardRepository, ZAK, ZPK);

    ISOMsg request = goodMacRequest("0000", securityModule);
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.CARD_ID, CARD_ID);
    ctx.put(TxnContextKeys.PAN, PAN);

    participant.prepare(1L, ctx);

    assertThat((String) ctx.get(TxnContextKeys.RESPONSE_CODE)).isEqualTo("75");
    Mockito.verify(cardRepository).blockForPin(CARD_ID);
  }

  @Test
  void should_prepare_and_reset_pin_try_count_on_correct_pin__MCN_503_AC1() throws Exception {
    JCESecurityModule securityModule = new JCESecurityModule(LMK_HEX);
    CardRepository cardRepository = Mockito.mock(CardRepository.class);
    when(cardRepository.findPvv(CARD_ID))
        .thenReturn(java.util.Optional.of(PvvCalculator.computePvv(PAN, "1234")));
    VerifySecurity participant = new VerifySecurity(securityModule, cardRepository, ZAK, ZPK);

    ISOMsg request = goodMacRequest("1234", securityModule);
    Context ctx = new Context();
    ctx.put(TxnContextKeys.REQUEST, request);
    ctx.put(TxnContextKeys.CARD_ID, CARD_ID);
    ctx.put(TxnContextKeys.PAN, PAN);

    int result = participant.prepare(1L, ctx);

    assertThat(result & PREPARED).isEqualTo(PREPARED);
    Mockito.verify(cardRepository).resetPinTryCount(CARD_ID);
    Mockito.verify(cardRepository, Mockito.never()).incrementPinTryCount(anyLong());
  }

  @Test
  void should_remove_de52_from_request_after_verification_regardless_of_outcome__MCN_503_AC3()
      throws Exception {
    JCESecurityModule securityModule = new JCESecurityModule(LMK_HEX);
    CardRepository cardRepository = Mockito.mock(CardRepository.class);
    when(cardRepository.findPvv(CARD_ID))
        .thenReturn(java.util.Optional.of(PvvCalculator.computePvv(PAN, "1234")));
    when(cardRepository.incrementPinTryCount(CARD_ID)).thenReturn(1);
    VerifySecurity participant = new VerifySecurity(securityModule, cardRepository, ZAK, ZPK);

    ISOMsg wrongPinRequest = goodMacRequest("0000", securityModule);
    Context ctxWrong = new Context();
    ctxWrong.put(TxnContextKeys.REQUEST, wrongPinRequest);
    ctxWrong.put(TxnContextKeys.CARD_ID, CARD_ID);
    ctxWrong.put(TxnContextKeys.PAN, PAN);
    participant.prepare(1L, ctxWrong);
    assertThat(wrongPinRequest.hasField(52)).isFalse();

    ISOMsg correctPinRequest = goodMacRequest("1234", securityModule);
    Context ctxCorrect = new Context();
    ctxCorrect.put(TxnContextKeys.REQUEST, correctPinRequest);
    ctxCorrect.put(TxnContextKeys.CARD_ID, CARD_ID);
    ctxCorrect.put(TxnContextKeys.PAN, PAN);
    participant.prepare(1L, ctxCorrect);
    assertThat(correctPinRequest.hasField(52)).isFalse();
  }

  private static ISOMsg baseFields() throws Exception {
    ISOMsg msg = new ISOMsg();
    msg.setPackager(packager());
    msg.setMTI("0200");
    msg.set(2, PAN);
    msg.set(3, "000000");
    msg.set(4, "000000250000");
    msg.set(7, "0921073208");
    msg.set(11, "000123");
    msg.set(32, "970499");
    msg.set(37, "626514000123");
    msg.set(41, "00000042");
    msg.set(42, "GOCPHO000000001");
    msg.set(49, "704");
    return msg;
  }

  /**
   * Builds a request with a genuine Retail MAC (under {@link #ZAK}) and a DE 52 encoding {@code
   * pin}.
   */
  private static ISOMsg goodMacRequest(String pin, JCESecurityModule securityModule)
      throws Exception {
    ISOMsg msg = baseFields();
    msg.set(52, HexFormat.of().formatHex(encryptPinBlock(buildPinBlock(pin, PAN))));

    byte[] packed = msg.pack();
    byte[] mac = securityModule.computeMac(packed, ZAK);
    msg.set(64, HexFormat.of().formatHex(mac));
    return msg;
  }

  /** ISO 9564-1 format 0, matching web-next's {@code buildPinBlock} (pinblock.ts). */
  private static byte[] buildPinBlock(String pin, String pan) {
    String pinField = ("0" + Integer.toHexString(pin.length()) + pin);
    StringBuilder pinFieldPadded = new StringBuilder(pinField);
    while (pinFieldPadded.length() < 16) {
      pinFieldPadded.append('F');
    }
    String panDigits = pan.substring(0, pan.length() - 1);
    panDigits = panDigits.substring(panDigits.length() - 12);
    String panField = "0000" + panDigits;

    byte[] result = new byte[8];
    for (int i = 0; i < 8; i++) {
      int hi =
          Character.digit(pinFieldPadded.charAt(i * 2), 16)
              ^ Character.digit(panField.charAt(i * 2), 16);
      int lo =
          Character.digit(pinFieldPadded.charAt(i * 2 + 1), 16)
              ^ Character.digit(panField.charAt(i * 2 + 1), 16);
      result[i] = (byte) ((hi << 4) | lo);
    }
    return result;
  }

  /** Test-only counterpart to {@link JCESecurityModule#decryptPinBlock}, same key expansion. */
  private static byte[] encryptPinBlock(byte[] clearBlock) throws Exception {
    byte[] tripleKey = new byte[24];
    System.arraycopy(ZPK, 0, tripleKey, 0, 16);
    System.arraycopy(ZPK, 0, tripleKey, 16, 8);
    Cipher cipher = Cipher.getInstance("DESede/ECB/NoPadding");
    cipher.init(Cipher.ENCRYPT_MODE, new SecretKeySpec(tripleKey, "DESede"));
    return cipher.doFinal(clearBlock);
  }
}
