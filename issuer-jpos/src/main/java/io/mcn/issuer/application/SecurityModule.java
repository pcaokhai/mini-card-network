package io.mcn.issuer.application;

/**
 * Keys are wrapped under an LMK-equivalent master key and never handled as clear {@code byte[]}
 * outside this port's implementation - see root {@code CLAUDE.md} §6 rule 2.
 */
public interface SecurityModule {
  byte[] wrapUnderLmk(byte[] clearKey);

  byte[] unwrap(byte[] keyUnderLmk);

  String computeKcv(byte[] clearKey);

  /**
   * ISO 9797-1 algorithm 3 (Retail MAC / X9.19) over {@code packedMessageExcludingMacField} under
   * {@code zak} - must be byte-identical to gateway-go's {@code JCEModule.ComputeMAC} for the same
   * input (MCN-503.md Ruling 1).
   */
  byte[] computeMac(byte[] packedMessageExcludingMacField, byte[] zak);

  /** Decrypts an ISO 9564-1 format 0 PIN block under {@code zpk} (2-key 3DES ECB, one block). */
  byte[] decryptPinBlock(byte[] pinBlockUnderZpk, byte[] zpk);
}
