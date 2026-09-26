package io.mcn.issuer.adapter.iso;

import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRow;
import io.mcn.issuer.application.SecurityModule;
import java.security.MessageDigest;
import java.util.Arrays;
import java.util.HexFormat;
import java.util.Optional;
import org.jpos.iso.ISOMsg;

/**
 * Handles the 0800 key-change advice (DE 70 = {@code 161}, docs/03 §7.3/§11, MCN-504): unwraps DE
 * 48's cryptogram under ZMK, stores the new key as {@code PENDING}, then immediately activates it
 * (this class's caller answers 0810 RC {@code 00}, which *is* {@code PARTNER_CONFIRM} from the
 * issuer's perspective - no separate confirm round-trip, per {@code MCN-504-ISS.md}'s Ruling 1). A
 * resent advice carrying the key that is already ACTIVE is acknowledged without a new row; one
 * carrying a key that was already RETIRED is a replay and is declined (SEC-G18).
 *
 * <p>Ruling (deviates from {@code MCN-504-ISS.md}'s Task 2): the plan's DE 123 key-type carrier
 * does not exist in this project's packager ({@code cfg/iso87ascii.xml} defines no field 123), and
 * adding one is a {@code contracts/} change out of scope for this branch (root {@code CLAUDE.md}
 * §9). Key type instead travels as a literal {@code "ZPK:"}/{@code "ZAK:"} prefix inside DE 48's
 * own value ({@code prefix + hex(cryptogramUnderZmk)}) - no packager change, no new field. The
 * gateway (initiator) side must emit DE 48 in this same shape.
 */
public final class ReceiveKeyChange {

  private final SecurityModule securityModule;
  private final KeyStoreRepository keyStoreRepository;
  private final AuditLogRepository auditLogRepository;
  private final byte[] zmk;
  private final String counterpartyId;

  public ReceiveKeyChange(
      SecurityModule securityModule,
      KeyStoreRepository keyStoreRepository,
      AuditLogRepository auditLogRepository,
      byte[] zmk,
      String counterpartyId) {
    this.securityModule = securityModule;
    this.keyStoreRepository = keyStoreRepository;
    this.auditLogRepository = auditLogRepository;
    this.zmk = zmk;
    this.counterpartyId = counterpartyId;
  }

  /** Returns {@code true} on success (caller responds RC 00), {@code false} on RC 96. */
  public boolean receive(ISOMsg request) {
    byte[] clearKey = null;
    try {
      String field48 = request.getString(48);
      int sep = field48 == null ? -1 : field48.indexOf(':');
      if (sep <= 0) {
        return false;
      }
      String keyType = field48.substring(0, sep);
      if (!"ZPK".equals(keyType) && !"ZAK".equals(keyType)) {
        return false;
      }
      byte[] cryptogramUnderZmk = HexFormat.of().parseHex(field48.substring(sep + 1));

      clearKey = securityModule.unwrapUnderKey(cryptogramUnderZmk, zmk);
      if (isAlreadyActive(keyType, clearKey)) {
        return true; // a resent advice (SEC-G15): 0810 00, no new row, the retired key unchanged
      }
      Optional<KeyStoreRow> replayed = retiredMatch(keyType, clearKey);
      if (replayed.isPresent()) {
        recordReplayRejected(keyType, replayed.get());
        return false; // SEC-G18: an old key never comes back - the caller answers 0810 RC 96
      }
      byte[] wrappedUnderLmk = securityModule.wrapUnderLmk(clearKey);
      String kcv = securityModule.computeKcv(clearKey);

      long id =
          keyStoreRepository.insert(
              new KeyStoreRow(
                  0,
                  keyType,
                  counterpartyId,
                  HexFormat.of().formatHex(wrappedUnderLmk),
                  kcv,
                  "PENDING",
                  null,
                  null,
                  null));
      try {
        keyStoreRepository.activate(id);
      } catch (RuntimeException e) {
        // SF2: a concurrent copy of this advice (the gateway resends an unanswered 0800, #121)
        // activated the same key first, so this activation hit the one-ACTIVE index (V8). If the
        // key it carries is now ACTIVE the change took effect: 00, and the winner has audited it.
        return isAlreadyActive(keyType, clearKey);
      }
      auditLogRepository.record(
          "issuer",
          "key_change.activated",
          "key_store",
          String.valueOf(id),
          null,
          "{\"keyType\":\""
              + keyType
              + "\",\"counterparty\":\""
              + counterpartyId
              + "\",\"newKcv\":\""
              + kcv
              + "\"}");
      return true;
    } catch (Exception e) {
      return false;
    } finally {
      if (clearKey != null) {
        Arrays.fill(clearKey, (byte) 0);
      }
      request.unset(48);
    }
  }

  /**
   * Whether {@code clearKey} is the ACTIVE key of that type already - the gateway resends an
   * unanswered 0800 with the same key (#121). Compares the keys themselves, in constant time: a
   * 3-byte KCV match alone could swallow a genuinely new key.
   */
  private boolean isAlreadyActive(String keyType, byte[] clearKey) {
    return keyStoreRepository
        .findActive(keyType, counterpartyId)
        .filter(row -> sameKey(row, clearKey))
        .isPresent();
  }

  /**
   * The RETIRED row holding {@code clearKey}, if any: a replayed cryptogram of an older key,
   * arriving after a later rotation, would otherwise re-activate a key already rotated out.
   */
  private Optional<KeyStoreRow> retiredMatch(String keyType, byte[] clearKey) {
    return keyStoreRepository.findRetired(keyType, counterpartyId).stream()
        .filter(row -> sameKey(row, clearKey))
        .findFirst();
  }

  /** Constant-time comparison with the row's key; the unwrapped copy is zeroed. */
  private boolean sameKey(KeyStoreRow row, byte[] clearKey) {
    byte[] stored = securityModule.unwrap(HexFormat.of().parseHex(row.keyUnderLmkHex()));
    try {
      return MessageDigest.isEqual(stored, clearKey);
    } finally {
      Arrays.fill(stored, (byte) 0);
    }
  }

  private void recordReplayRejected(String keyType, KeyStoreRow retired) {
    auditLogRepository.record(
        "issuer",
        "key_change.replay_rejected",
        "key_store",
        String.valueOf(retired.id()),
        null,
        "{\"keyType\":\""
            + keyType
            + "\",\"counterparty\":\""
            + counterpartyId
            + "\",\"reason\":\"key equals a RETIRED key\"}");
  }
}
