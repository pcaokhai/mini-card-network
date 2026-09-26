package io.mcn.issuer.adapter.txn;

import io.mcn.issuer.application.SessionKeys;
import java.util.Map;
import java.util.Optional;

/** Test {@link SessionKeys} over fixed clear keys; hands out copies, as the real one does. */
record FixedSessionKeys(Map<String, byte[]> active, Map<String, byte[]> retired)
    implements SessionKeys {

  static FixedSessionKeys of(byte[] zak, byte[] zpk) {
    return new FixedSessionKeys(Map.of("ZAK", zak, "ZPK", zpk), Map.of());
  }

  FixedSessionKeys withRetired(String keyType, byte[] key) {
    return new FixedSessionKeys(active, Map.of(keyType, key));
  }

  @Override
  public byte[] active(String keyType) {
    return active.get(keyType).clone();
  }

  @Override
  public Optional<byte[]> recentlyRetired(String keyType) {
    return Optional.ofNullable(retired.get(keyType)).map(byte[]::clone);
  }
}
