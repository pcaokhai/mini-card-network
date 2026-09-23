package io.mcn.issuer.application;

/**
 * Keys are wrapped under an LMK-equivalent master key and never handled as clear {@code byte[]}
 * outside this port's implementation - see root {@code CLAUDE.md} §6 rule 2.
 */
public interface SecurityModule {
  byte[] wrapUnderLmk(byte[] clearKey);

  byte[] unwrap(byte[] keyUnderLmk);

  String computeKcv(byte[] clearKey);
}
