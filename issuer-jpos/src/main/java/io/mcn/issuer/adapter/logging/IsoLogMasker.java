package io.mcn.issuer.adapter.logging;

import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;

/** Formats an ISOMsg for logging with PAN masked and other sensitive DEs redacted (docs/02 §7.6, docs/03 §3). */
public final class IsoLogMasker {
  // DE -> label from packager-spec.yaml's `sensitive:` tag, for fields that are not PAN-shaped
  // digit runs and so cannot use PanMasker's regex (key material, PIN block, EMV TLV).
  private static final java.util.Map<Integer, String> REDACT_ONLY =
      java.util.Map.of(48, "key-material", 52, "pin-block", 55, "emv");

  private IsoLogMasker() {}

  public static String maskedDump(ISOMsg msg) throws ISOException {
    StringBuilder sb = new StringBuilder("MTI=").append(msg.getMTI());
    for (int i = 2; i <= 128; i++) {
      if (!msg.hasField(i)) continue;
      String value = msg.getString(i);
      sb.append(' ').append(i).append(": ");
      if (REDACT_ONLY.containsKey(i)) {
        sb.append("[MASKED ").append(REDACT_ONLY.get(i)).append(']');
      } else if (i == 2) {
        sb.append(PanMasker.mask(value));
      } else {
        sb.append(value);
      }
    }
    return sb.toString();
  }
}
