package io.mcn.issuer.adapter.txn;

import java.security.SecureRandom;

/**
 * 6-character uppercase alphanumeric auth code (DE 38) generator. Uniqueness is probabilistic (36^6
 * ≈ 2.2 billion combinations) - acceptable for a teaching lab's transaction volume; add a DB-backed
 * sequence/dedupe check only if a real collision is ever observed (YAGNI).
 */
public class AuthCodeGenerator {
  private static final String ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
  private static final int LENGTH = 6;

  private final SecureRandom random = new SecureRandom();

  public String generate() {
    StringBuilder sb = new StringBuilder(LENGTH);
    for (int i = 0; i < LENGTH; i++) {
      sb.append(ALPHABET.charAt(random.nextInt(ALPHABET.length())));
    }
    return sb.toString();
  }
}
