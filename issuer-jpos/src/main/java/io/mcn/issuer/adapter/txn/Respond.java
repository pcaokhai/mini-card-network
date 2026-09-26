package io.mcn.issuer.adapter.txn;

import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import io.mcn.issuer.application.SecurityModule;
import java.io.Serializable;
import java.util.HexFormat;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.ISOSource;
import org.jpos.transaction.AbortParticipant;
import org.jpos.transaction.Context;

/**
 * Builds the 0210 response and sends it via the {@link ISOSource} stored in the context. A
 * duplicate replays the stored response's DE 38/39/4/54 verbatim (docs/03 §7.5) instead of
 * rebuilding them. Implements {@link AbortParticipant} and overrides {@code abort()} for the same
 * reason as {@code LogAndOutbox}: a decline earlier in the chain makes the whole transaction abort,
 * and every declined request still needs a response sent.
 *
 * <p>Every response is signed with the Retail MAC under the ZAK before it leaves (docs/03 §11,
 * {@code contracts/iso8583/vectors/0210-approved.json} carries DE 64): the gateway treats a 0210
 * without a valid MAC as a MAC failure and declines with RC 96.
 */
public class Respond implements AbortParticipant, Configurable {

  private SecurityModule securityModule;
  private byte[] zak;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public Respond() {}

  public Respond(SecurityModule securityModule, byte[] zak) {
    this.securityModule = securityModule;
    this.zak = zak;
  }

  @Override
  public void setConfiguration(Configuration cfg) {
    this.securityModule = new JCESecurityModule(env("LMK_TEST_VALUE_HEX"));
    this.zak = HexFormat.of().parseHex(env("ZAK_HEX"));
  }

  private static String env(String name) {
    String value = System.getenv(name);
    return value != null ? value : System.getProperty(name);
  }

  @Override
  public int prepare(long id, Serializable context) {
    return PREPARED;
  }

  @Override
  public void commit(long id, Serializable context) {
    send(context);
  }

  @Override
  public void abort(long id, Serializable context) {
    send(context);
  }

  private void send(Serializable context) {
    Context ctx = (Context) context;
    ISOSource source = ctx.get(TxnContextKeys.SOURCE);
    if (source == null) {
      return;
    }
    try {
      source.send(signedResponse(ctx));
    } catch (ISOException | java.io.IOException e) {
      throw new IllegalStateException("failed to send authorization response", e);
    }
  }

  /** {@link #buildResponse} with its MAC attached: what actually goes on the wire. */
  ISOMsg signedResponse(Context ctx) throws ISOException {
    ISOMsg response = buildResponse(ctx);
    int macField = hasSecondaryBitmapFields(response) ? 128 : 64;
    response.unset(64);
    response.unset(128);
    response.set(macField, securityModule.computeMac(response.pack(), zak));
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

  ISOMsg buildResponse(Context ctx) throws ISOException {
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);
    ISOMsg response = (ISOMsg) request.clone();
    response.setResponseMTI();

    if (Boolean.TRUE.equals(ctx.<Boolean>get(TxnContextKeys.IS_DUPLICATE))) {
      ISOMsg stored = ctx.get(TxnContextKeys.STORED_RESPONSE);
      if (stored != null) {
        copyIfPresent(stored, response, 4);
        copyIfPresent(stored, response, 38);
        copyIfPresent(stored, response, 39);
        copyIfPresent(stored, response, 54);
        return response;
      }
    }

    String responseCode = ctx.get(TxnContextKeys.RESPONSE_CODE);
    response.set(39, responseCode);
    String authCode = ctx.get(TxnContextKeys.AUTH_CODE);
    if (authCode != null) {
      response.set(38, authCode);
    }
    Long balance = ctx.get(TxnContextKeys.BALANCE);
    if (balance != null && "00".equals(responseCode)) {
      response.set(54, de54(ctx.get(TxnContextKeys.BALANCE_CURRENCY), balance));
    }
    byte[] arpc = ctx.get(TxnContextKeys.EMV_ARPC);
    if (arpc != null && ("00".equals(responseCode) || "10".equals(responseCode))) {
      response.set(55, buildArpcTlv(arpc));
    }
    return response;
  }

  /**
   * The balance sub-format the gateway parses (docs/03 §3 leaves DE 54's layout to the
   * implementation): currency (3) + C/D sign (1) + minor units (12).
   */
  private static String de54(String currency, long balance) {
    return currency + (balance < 0 ? "D" : "C") + String.format("%012d", Math.abs(balance));
  }

  /**
   * Tag 91 (ARPC), simple-TLV, single-byte length - matches {@code EmvTlvParser}'s own encoding.
   */
  private static byte[] buildArpcTlv(byte[] arpc) {
    byte[] tlv = new byte[2 + arpc.length];
    tlv[0] = (byte) 0x91;
    tlv[1] = (byte) arpc.length;
    System.arraycopy(arpc, 0, tlv, 2, arpc.length);
    return tlv;
  }

  private static void copyIfPresent(ISOMsg from, ISOMsg to, int field) throws ISOException {
    if (from.hasField(field)) {
      to.set(field, from.getString(field));
    }
  }
}
