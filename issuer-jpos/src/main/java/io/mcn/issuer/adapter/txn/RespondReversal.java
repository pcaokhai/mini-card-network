package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import io.mcn.issuer.adapter.crypto.KeyStoreSessionKeys;
import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.application.SecurityModule;
import io.mcn.issuer.application.SessionKeys;
import java.io.Serializable;
import java.util.HexFormat;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.ISOSource;
import org.jpos.transaction.AbortParticipant;
import org.jpos.transaction.Context;
import org.jpos.util.Destroyable;

/**
 * Always sends 0430 (docs/03 §7.3: "Advices cannot be declined") - never inspects {@code
 * RESPONSE_CODE}/{@code IS_DUPLICATE}, unlike {@code Respond}. DE 39 is "00" once the reversal is
 * recorded and "96" when the transaction aborted, so the acquirer repeats it. {@code
 * setResponseMTI()} maps both 0420 and 0421 (repeat) to 0430. Implements {@link AbortParticipant}
 * for the same reason {@code Respond} does: every member of an aborting transaction still needs its
 * response sent.
 */
public class RespondReversal implements AbortParticipant, Configurable, Destroyable {

  private static final String ACKNOWLEDGED = "00";
  private static final String SYSTEM_MALFUNCTION = "96";

  // docs/03 §3 - the counterparty ReceiveKeyChange writes key_store rows under.
  private static final String LAB_ACQUIRER_ID = "970499";

  private SecurityModule securityModule;
  private SessionKeys sessionKeys;
  private HikariDataSource dataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public RespondReversal() {}

  public RespondReversal(SecurityModule securityModule, SessionKeys sessionKeys) {
    this.securityModule = securityModule;
    this.sessionKeys = sessionKeys;
  }

  /** MACs every 0430 under the ACTIVE ZAK in {@code key_store} (docs/03 §4, SEC-G15). */
  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.dataSource = TxnDataSource.fromConfig(cfg);
    this.securityModule = new JCESecurityModule(env("LMK_TEST_VALUE_HEX"));
    var keys =
        new KeyStoreSessionKeys(
            new KeyStoreRepository(dataSource), securityModule, LAB_ACQUIRER_ID);
    keys.ensureActive("ZAK", HexFormat.of().parseHex(env("ZAK_HEX")));
    this.sessionKeys = keys;
  }

  private static String env(String name) {
    String value = System.getenv(name);
    return value != null ? value : System.getProperty(name);
  }

  @Override
  public void destroy() {
    if (dataSource != null) {
      dataSource.close();
    }
  }

  @Override
  public int prepare(long id, Serializable context) {
    return PREPARED;
  }

  @Override
  public void commit(long id, Serializable context) {
    send(context, ACKNOWLEDGED);
  }

  @Override
  public void abort(long id, Serializable context) {
    // Not recorded, so still owed: anything but "00" makes the acquirer's SAF repeat it.
    send(context, SYSTEM_MALFUNCTION);
  }

  private void send(Serializable context, String responseCode) {
    Context ctx = (Context) context;
    ISOSource source = ctx.get(TxnContextKeys.SOURCE);
    if (source == null) {
      return;
    }
    try {
      source.send(signedResponse(ctx, responseCode));
    } catch (ISOException | java.io.IOException e) {
      throw new IllegalStateException("failed to send reversal response", e);
    }
  }

  /** {@link #buildResponse} with its MAC attached: what actually goes on the wire. */
  ISOMsg signedResponse(Context ctx, String responseCode) throws ISOException {
    return new ResponseMac(securityModule, sessionKeys).sign(buildResponse(ctx, responseCode));
  }

  ISOMsg buildResponse(Context ctx) throws ISOException {
    return buildResponse(ctx, ACKNOWLEDGED);
  }

  private ISOMsg buildResponse(Context ctx, String responseCode) throws ISOException {
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);
    ISOMsg response = (ISOMsg) request.clone();
    response.setResponseMTI();
    // The clone carries the 0420's reason code in DE 39; the 0430's DE 39 is the acknowledgement
    // (docs/03 §3), and the acquirer only treats "00" as delivered.
    response.set(39, responseCode);
    return response;
  }
}
