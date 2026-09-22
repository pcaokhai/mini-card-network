package io.mcn.issuer.adapter.crypto;

import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.SecureRandom;
import java.util.Arrays;
import java.util.HexFormat;
import javax.crypto.Cipher;
import javax.crypto.Mac;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;

/**
 * Simulates "keys under LMK" for the lab: a real deployment would use an HSM; here a single
 * app-held master key (from env) plays that role. Never store PAN/CVV/PIN in clear.
 */
public class CardCrypto {

  private static final int GCM_TAG_BITS = 128;
  private static final int NONCE_BYTES = 12;

  private final SecretKeySpec encryptionKey;
  private final SecretKeySpec hmacKey;
  private final SecureRandom random = new SecureRandom();

  public CardCrypto(String encryptionKeyHex, String hmacKeyHex) {
    this.encryptionKey = new SecretKeySpec(HexFormat.of().parseHex(encryptionKeyHex), "AES");
    this.hmacKey = new SecretKeySpec(HexFormat.of().parseHex(hmacKeyHex), "HmacSHA256");
  }

  public byte[] encrypt(String pan) {
    try {
      byte[] nonce = new byte[NONCE_BYTES];
      random.nextBytes(nonce);
      Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
      cipher.init(Cipher.ENCRYPT_MODE, encryptionKey, new GCMParameterSpec(GCM_TAG_BITS, nonce));
      byte[] ciphertext = cipher.doFinal(pan.getBytes(StandardCharsets.UTF_8));
      return ByteBuffer.allocate(nonce.length + ciphertext.length)
          .put(nonce)
          .put(ciphertext)
          .array();
    } catch (Exception e) {
      throw new IllegalStateException("PAN encryption failed", e);
    }
  }

  public String decrypt(byte[] encrypted) {
    try {
      byte[] nonce = Arrays.copyOfRange(encrypted, 0, NONCE_BYTES);
      byte[] ciphertext = Arrays.copyOfRange(encrypted, NONCE_BYTES, encrypted.length);
      Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
      cipher.init(Cipher.DECRYPT_MODE, encryptionKey, new GCMParameterSpec(GCM_TAG_BITS, nonce));
      return new String(cipher.doFinal(ciphertext), StandardCharsets.UTF_8);
    } catch (Exception e) {
      throw new IllegalStateException("PAN decryption failed", e);
    }
  }

  public byte[] hash(String pan) {
    try {
      Mac mac = Mac.getInstance("HmacSHA256");
      mac.init(hmacKey);
      return mac.doFinal(pan.getBytes(StandardCharsets.UTF_8));
    } catch (Exception e) {
      throw new IllegalStateException("PAN hashing failed", e);
    }
  }
}
