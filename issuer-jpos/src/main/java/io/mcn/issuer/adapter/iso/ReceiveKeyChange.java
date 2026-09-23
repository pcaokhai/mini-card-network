package io.mcn.issuer.adapter.iso;

import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRow;
import io.mcn.issuer.application.SecurityModule;
import java.util.Arrays;
import java.util.HexFormat;
import org.jpos.iso.ISOMsg;

/**
 * Handles the 0800 key-change advice (DE 70 = {@code 161}, docs/03 §7.3/§11, MCN-504): unwraps DE
 * 48's cryptogram under ZMK, stores the new key as {@code PENDING}, then immediately activates it
 * (this class's caller answers 0810 RC {@code 00}, which *is* {@code PARTNER_CONFIRM} from the
 * issuer's perspective - no separate confirm round-trip, per {@code MCN-504-ISS.md}'s Ruling 1).
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
      keyStoreRepository.activate(id);
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
}
