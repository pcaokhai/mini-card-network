package io.mcn.issuer.adapter.crypto;

import io.mcn.issuer.application.SecurityModule;
import java.nio.ByteBuffer;
import java.security.SecureRandom;
import java.util.Arrays;
import java.util.HexFormat;
import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.IvParameterSpec;
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
    return unwrapUnder(keyUnderLmk, lmk);
  }

  @Override
  public byte[] unwrapUnderKey(byte[] cryptogram, byte[] key) {
    return unwrapUnder(cryptogram, new SecretKeySpec(key, "AES"));
  }

  private static byte[] unwrapUnder(byte[] cryptogram, SecretKeySpec key) {
    try {
      byte[] nonce = Arrays.copyOfRange(cryptogram, 0, NONCE_BYTES);
      byte[] ciphertext = Arrays.copyOfRange(cryptogram, NONCE_BYTES, cryptogram.length);
      Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
      cipher.init(Cipher.DECRYPT_MODE, key, new GCMParameterSpec(GCM_TAG_BITS, nonce));
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

  /**
   * Ported byte-for-byte from gateway-go's {@code JCEModule.ComputeMAC} (MCN-503.md Ruling 1):
   * split {@code zak} into k1/k2 (single-length DES each), zero-pad the message to a DES block
   * multiple, CBC-encrypt under k1 with a zero IV, decrypt the final block under k2, re-encrypt
   * that block under k1 - those 8 bytes are the MAC.
   */
  @Override
  public byte[] computeMac(byte[] packedMessageExcludingMacField, byte[] zak) {
    if (zak.length != 16) {
      throw new IllegalArgumentException("ZAK must be 16 bytes (double-length DES)");
    }
    try {
      SecretKeySpec k1 = new SecretKeySpec(Arrays.copyOfRange(zak, 0, 8), "DES");
      SecretKeySpec k2 = new SecretKeySpec(Arrays.copyOfRange(zak, 8, 16), "DES");
      byte[] padded = zeroPad(packedMessageExcludingMacField, 8);

      Cipher cbcEncrypt = Cipher.getInstance("DES/CBC/NoPadding");
      cbcEncrypt.init(Cipher.ENCRYPT_MODE, k1, new IvParameterSpec(new byte[8]));
      byte[] encrypted = cbcEncrypt.doFinal(padded);
      byte[] lastBlock = Arrays.copyOfRange(encrypted, encrypted.length - 8, encrypted.length);

      Cipher ecbDecrypt = Cipher.getInstance("DES/ECB/NoPadding");
      ecbDecrypt.init(Cipher.DECRYPT_MODE, k2);
      byte[] intermediate = ecbDecrypt.doFinal(lastBlock);

      Cipher ecbEncrypt = Cipher.getInstance("DES/ECB/NoPadding");
      ecbEncrypt.init(Cipher.ENCRYPT_MODE, k1);
      return ecbEncrypt.doFinal(intermediate);
    } catch (Exception e) {
      throw new IllegalStateException("MAC computation failed", e);
    }
  }

  private static byte[] zeroPad(byte[] data, int blockSize) {
    if (data.length % blockSize == 0) {
      return data;
    }
    byte[] padded = Arrays.copyOf(data, data.length + (blockSize - data.length % blockSize));
    return padded;
  }

  /**
   * 2-key 3DES (K1,K2,K1 expansion) ECB decrypt of one 8-byte block, matching gateway-go's
   * desCipher(16).
   */
  @Override
  public byte[] decryptPinBlock(byte[] pinBlockUnderZpk, byte[] zpk) {
    if (zpk.length != 16) {
      throw new IllegalArgumentException("ZPK must be 16 bytes (double-length 3DES)");
    }
    try {
      byte[] tripleKey = new byte[24];
      System.arraycopy(zpk, 0, tripleKey, 0, 16);
      System.arraycopy(zpk, 0, tripleKey, 16, 8);
      Cipher cipher = Cipher.getInstance("DESede/ECB/NoPadding");
      cipher.init(Cipher.DECRYPT_MODE, new SecretKeySpec(tripleKey, "DESede"));
      return cipher.doFinal(pinBlockUnderZpk);
    } catch (Exception e) {
      throw new IllegalStateException("PIN block decryption failed", e);
    }
  }
}
