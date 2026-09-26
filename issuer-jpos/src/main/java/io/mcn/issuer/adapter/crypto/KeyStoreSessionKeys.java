package io.mcn.issuer.adapter.crypto;

import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRow;
import io.mcn.issuer.application.SecurityModule;
import io.mcn.issuer.application.SessionKeys;
import java.util.Arrays;
import java.util.HexFormat;
import java.util.Optional;

/**
 * {@link SessionKeys} read from {@code key_store} on every use and unwrapped from their LMK
 * cryptogram, so the key {@code ReceiveKeyChange} activates is the one the next message uses, in
 * every Q2 bean, with no shared in-memory state to keep in step (SEC-G15).
 *
 * <p>ponytail: one indexed read and an AES-GCM unwrap per message and key; add a short TTL cache
 * (dropped on a MAC mismatch) if this ever shows in the issuer's p99.
 */
public final class KeyStoreSessionKeys implements SessionKeys {
  private final KeyStoreRepository keyStore;
  private final SecurityModule securityModule;
  private final String counterparty;

  public KeyStoreSessionKeys(
      KeyStoreRepository keyStore, SecurityModule securityModule, String counterparty) {
    this.keyStore = keyStore;
    this.securityModule = securityModule;
    this.counterparty = counterparty;
  }

  /**
   * Seeds {@code clearKey} (the environment's initial key) as ACTIVE when none is, stored only as
   * its LMK cryptogram; an existing ACTIVE key (a rotation's) always wins. Zeroes {@code clearKey}.
   */
  public void ensureActive(String keyType, byte[] clearKey) {
    try {
      keyStore.ensureActive(
          new KeyStoreRow(
              0,
              keyType,
              counterparty,
              HexFormat.of().formatHex(securityModule.wrapUnderLmk(clearKey)),
              securityModule.computeKcv(clearKey),
              "ACTIVE",
              null,
              null,
              null));
    } finally {
      Arrays.fill(clearKey, (byte) 0);
    }
  }

  @Override
  public byte[] active(String keyType) {
    return keyStore
        .findActive(keyType, counterparty)
        .map(this::unwrap)
        .orElseThrow(() -> new IllegalStateException("no ACTIVE " + keyType + " in key_store"));
  }

  @Override
  public Optional<byte[]> recentlyRetired(String keyType) {
    return keyStore
        .findRecentlyRetired(keyType, counterparty, KeyStoreRepository.DUAL_KEY_WINDOW)
        .map(this::unwrap);
  }

  private byte[] unwrap(KeyStoreRow row) {
    return securityModule.unwrap(HexFormat.of().parseHex(row.keyUnderLmkHex()));
  }
}
