package io.mcn.issuer.adapter.crypto;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.Test;

class CardCryptoTest {
  private final CardCrypto crypto =
      new CardCrypto(
          "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
          "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210");

  @Test
  void encryptDecryptRoundTrips() {
    byte[] encrypted = crypto.encrypt("9704360000004417");
    assertThat(crypto.decrypt(encrypted)).isEqualTo("9704360000004417");
  }

  @Test
  void sameEncryptionCallsProduceDifferentCiphertext() {
    assertThat(crypto.encrypt("9704360000004417")).isNotEqualTo(crypto.encrypt("9704360000004417"));
  }

  @Test
  void hashIsDeterministicAndDiffersFromEncryption() {
    byte[] h1 = crypto.hash("9704360000004417");
    byte[] h2 = crypto.hash("9704360000004417");
    assertThat(h1).isEqualTo(h2);
    assertThat(h1).isNotEqualTo(crypto.encrypt("9704360000004417"));
  }
}
