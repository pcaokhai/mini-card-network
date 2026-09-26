package io.mcn.issuer.adapter.txn;

import io.mcn.issuer.application.SecurityModule;
import io.mcn.issuer.application.SessionKeys;
import java.util.Arrays;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;

/**
 * MACs an issuer response under the ACTIVE ZAK (docs/03 §4, §11): in DE 128 when a field above 64
 * sets the secondary bitmap (a 0430 echoes DE 90), else DE 64 - the gateway's own rule (MCN-502
 * Ruling 2). Any MAC the response inherited from its request is dropped first.
 */
public final class ResponseMac {
  private final SecurityModule securityModule;
  private final SessionKeys sessionKeys;

  public ResponseMac(SecurityModule securityModule, SessionKeys sessionKeys) {
    this.securityModule = securityModule;
    this.sessionKeys = sessionKeys;
  }

  public ISOMsg sign(ISOMsg response) throws ISOException {
    response.unset(64);
    response.unset(128);
    int macField = hasSecondaryBitmapFields(response) ? 128 : 64;
    byte[] zak = sessionKeys.active("ZAK");
    try {
      response.set(macField, securityModule.computeMac(response.pack(), zak));
    } finally {
      Arrays.fill(zak, (byte) 0);
    }
    return response;
  }

  private static boolean hasSecondaryBitmapFields(ISOMsg msg) {
    for (int field = 65; field <= 127; field++) {
      if (msg.hasField(field)) {
        return true;
      }
    }
    return false;
  }
}
