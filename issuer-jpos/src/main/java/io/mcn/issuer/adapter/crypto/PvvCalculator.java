package io.mcn.issuer.adapter.crypto;

import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.util.HexFormat;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

/**
 * Simulated PVV (PIN Verification Value) for the lab - not a real IBM 3624/Visa PVV algorithm, same
 * "simulated in the lab" pattern docs/03 §11 already establishes for ARQC: a keyed HMAC truncated
 * to 4 decimal digits, under a fixed, documented, non-secret PVK (MCN-503.md's plan). {@link
 * io.mcn.issuer.adapter.seed.SeedLoader} and {@link io.mcn.issuer.adapter.txn.VerifySecurity} both
 * call this same function so seeded and verified PVVs never drift apart.
 */
public final class PvvCalculator {
  private PvvCalculator() {}

  private static final byte[] SIMULATOR_PVK =
      HexFormat.of().parseHex("0F0F0F0F0F0F0F0F0F0F0F0F0F0F0F0F");
  private static final BigInteger TEN_THOUSAND = BigInteger.valueOf(10_000);

  public static String computePvv(String pan, String pin) {
    try {
      Mac mac = Mac.getInstance("HmacSHA256");
      mac.init(new SecretKeySpec(SIMULATOR_PVK, "HmacSHA256"));
      byte[] digest = mac.doFinal((pan + pin).getBytes(StandardCharsets.UTF_8));
      long value = new BigInteger(1, digest).mod(TEN_THOUSAND).longValue();
      return String.format("%04d", value);
    } catch (Exception e) {
      throw new IllegalStateException("PVV computation failed", e);
    }
  }
}
