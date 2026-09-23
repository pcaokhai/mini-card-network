package io.mcn.issuer.adapter.crypto;

import io.mcn.issuer.application.SecurityModule;
import java.nio.ByteBuffer;
import java.security.SecureRandom;
import java.util.Arrays;
import java.util.HexFormat;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;

/**
 * Simulates "keys under LMK" for the lab, same pattern as {@link CardCrypto}: a single app-held
 * master key (from env) plays the LMK's role. Never store or return a clear key.
 */
public class JCESecurityModule implements SecurityModule {

  private static final int GCM_TAG_BITS = 128;
  private static final int NONCE_BYTES = 12;
  private static final byte[] ZERO_BLOCK = new byte[16];

  private final SecretKeySpec lmk;
  private final SecureRandom random = new SecureRandom();

  public JCESecurityModule(String lmkHex) {
    if (lmkHex == null || lmkHex.isBlank()) {
      throw new IllegalArgumentException("LMK test value is required");
    }
    this.lmk = new SecretKeySpec(HexFormat.of().parseHex(lmkHex), "AES");
  }

  @Override
  public byte[] wrapUnderLmk(byte[] clearKey) {
    try {
      byte[] nonce = new byte[NONCE_BYTES];
      random.nextBytes(nonce);
      Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
      cipher.init(Cipher.ENCRYPT_MODE, lmk, new GCMParameterSpec(GCM_TAG_BITS, nonce));
      byte[] ciphertext = cipher.doFinal(clearKey);
      return ByteBuffer.allocate(nonce.length + ciphertext.length)
          .put(nonce)
          .put(ciphertext)
          .array();
    } catch (Exception e) {
      throw new IllegalStateException("key wrap failed", e);
    }
  }

  @Override
  public byte[] unwrap(byte[] keyUnderLmk) {
    try {
      byte[] nonce = Arrays.copyOfRange(keyUnderLmk, 0, NONCE_BYTES);
      byte[] ciphertext = Arrays.copyOfRange(keyUnderLmk, NONCE_BYTES, keyUnderLmk.length);
      Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
      cipher.init(Cipher.DECRYPT_MODE, lmk, new GCMParameterSpec(GCM_TAG_BITS, nonce));
      return cipher.doFinal(ciphertext);
    } catch (Exception e) {
      throw new IllegalStateException("key unwrap failed", e);
    }
  }

  @Override
  public String computeKcv(byte[] clearKey) {
    try {
      Cipher cipher = Cipher.getInstance("AES/ECB/NoPadding");
      cipher.init(Cipher.ENCRYPT_MODE, new SecretKeySpec(clearKey, "AES"));
      byte[] encryptedZeroBlock = cipher.doFinal(ZERO_BLOCK);
      return HexFormat.of().withUpperCase().formatHex(Arrays.copyOfRange(encryptedZeroBlock, 0, 3));
    } catch (Exception e) {
      throw new IllegalStateException("KCV computation failed", e);
    }
  }
}
