package io.mcn.issuer.application;

import java.util.Optional;

/**
 * The working keys shared with the acquirer (ZAK for MACs, ZPK for PIN blocks) as they stand right
 * now, so a key change (DE 70 = 161) takes effect on the next message (SEC-G15). Every method
 * returns a fresh clear copy that the caller owns and zeroes after use; the key only exists at rest
 * as a cryptogram under the LMK.
 */
public interface SessionKeys {
  /** The ACTIVE key of {@code keyType}. */
  byte[] active(String keyType);

  /**
   * The key of {@code keyType} retired by the latest change, if that was within the dual-key window
   * (docs/03 §9: 5 minutes) and the key had actually been active.
   */
  Optional<byte[]> recentlyRetired(String keyType);
}
