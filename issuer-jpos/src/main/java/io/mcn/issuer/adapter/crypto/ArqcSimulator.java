package io.mcn.issuer.adapter.crypto;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Arrays;
import java.util.HexFormat;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

/**
 * Simulated ARQC/ARPC for the lab - not a real EMV cryptogram derivation, same "simulated in the
 * lab" pattern as {@link PvvCalculator}: a keyed HMAC over the transaction data, truncated to 8
 * bytes, under a fixed, documented, non-secret simulator key. Never wired to any real
 * key-management path (LMK/ZAK/ZPK).
 */
public final class ArqcSimulator {

  private static final byte[] SIMULATOR_ARQC_KEY =
      HexFormat.of().parseHex("A5A5A5A5A5A5A5A5A5A5A5A5A5A5A5A5");
  private static final int MAC_LENGTH = 8;

  public boolean verifyArqc(byte[] arqc, String pan, int atc, byte[] unpredictableNumber) {
    byte[] expected = computeArqcForTest(pan, atc, unpredictableNumber);
    return MessageDigest.isEqual(arqc, expected);
  }

  public byte[] computeArpc(byte[] arqc) {
    return hmacTruncated(SIMULATOR_ARQC_KEY, arqc);
  }

  /** Test-only helper mirroring the real HMAC construction, so tests build a known-good vector. */
  public byte[] computeArqcForTest(String pan, int atc, byte[] unpredictableNumber) {
    String data = pan + atc + HexFormat.of().formatHex(unpredictableNumber);
    return hmacTruncated(SIMULATOR_ARQC_KEY, data.getBytes(StandardCharsets.UTF_8));
  }

  private static byte[] hmacTruncated(byte[] key, byte[] data) {
    try {
      Mac mac = Mac.getInstance("HmacSHA256");
      mac.init(new SecretKeySpec(key, "HmacSHA256"));
      byte[] digest = mac.doFinal(data);
      return Arrays.copyOf(digest, MAC_LENGTH);
    } catch (Exception e) {
      throw new IllegalStateException("ARQC/ARPC computation failed", e);
    }
  }
}
