package io.mcn.issuer.adapter.iso;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import io.mcn.issuer.adapter.crypto.KeyStoreSessionKeys;
import io.mcn.issuer.adapter.persistence.AcquirerLinkRepository;
import io.mcn.issuer.adapter.persistence.JdbcAcquirerLinkRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.txn.ResponseMac;
import io.mcn.issuer.adapter.txn.TxnContextKeys;
import java.util.HexFormat;
import org.flywaydb.core.Flyway;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.ISORequestListener;
import org.jpos.iso.ISOSource;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionManager;
import org.jpos.util.Log;
import org.jpos.util.NameRegistrar;

/**
 * {@code ISORequestListener} for reversal advice (MTI class 04) requests (MCN-402), wired in {@code
 * 30_iso_server.xml} alongside {@link AuthorizationListener}. Mirrors {@code
 * AuthorizationListener}'s structure (not-signed-on short-circuit with RC 91, otherwise queue and
 * return) but dispatches into {@code reversal-txn-mgr} instead of {@code iso-txn-mgr}.
 */
public final class ReversalListener extends Log implements ISORequestListener, Configurable {

  private static final String LAB_ACQUIRER_ID = "970499"; // docs/03 §3

  private AcquirerLinkRepository links;
  private String txnMgrName;
  private ResponseMac responseMac;

  public ReversalListener() {}

  public ReversalListener(
      AcquirerLinkRepository links, String txnMgrName, ResponseMac responseMac) {
    this.links = links;
    this.txnMgrName = txnMgrName;
    this.responseMac = responseMac;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    HikariConfig hikariConfig = new HikariConfig();
    hikariConfig.setJdbcUrl(cfg.get("jdbc-url"));
    hikariConfig.setUsername(cfg.get("jdbc-user"));
    hikariConfig.setPassword(cfg.get("jdbc-password"));
    hikariConfig.setMaximumPoolSize(2);
    HikariDataSource dataSource = new HikariDataSource(hikariConfig);
    Flyway.configure().dataSource(dataSource).load().migrate();
    this.links = new JdbcAcquirerLinkRepository(dataSource);
    this.txnMgrName = cfg.get("txn-mgr-name", "reversal-txn-mgr");
    // The 0430s this listener answers itself are MACed like RespondReversal's (NET-G19).
    var securityModule = new JCESecurityModule(env("LMK_TEST_VALUE_HEX"));
    var keys =
        new KeyStoreSessionKeys(
            new KeyStoreRepository(dataSource), securityModule, LAB_ACQUIRER_ID);
    keys.ensureActive("ZAK", HexFormat.of().parseHex(env("ZAK_HEX")));
    this.responseMac = new ResponseMac(securityModule, keys);
  }

  private static String env(String name) {
    String value = System.getenv(name);
    return value != null ? value : System.getProperty(name);
  }

  @Override
  public boolean process(ISOSource source, ISOMsg request) {
    try {
      String mtiClass = request.getMTI().substring(0, 2);
      if (!"04".equals(mtiClass)) {
        return false; // not ours
      }

      if (links.findStatus(LAB_ACQUIRER_ID).filter("SIGNED_ON"::equals).isEmpty()) {
        respond(source, request, "91");
        return true;
      }

      Context ctx = new Context();
      ctx.put(TxnContextKeys.REQUEST, request);
      ctx.put(TxnContextKeys.SOURCE, source);
      ctx.put(TxnContextKeys.ACQUIRER_ID, LAB_ACQUIRER_ID);
      transactionManager().queue(ctx);
      return true;
    } catch (Exception e) {
      warn("failed to accept reversal request, responding 0430", e);
      respondAlways0430(source, request);
      return true;
    }
  }

  private TransactionManager transactionManager() {
    TransactionManager tm = NameRegistrar.getIfExists(txnMgrName);
    if (tm == null) {
      throw new IllegalStateException("TransactionManager '" + txnMgrName + "' is not registered");
    }
    return tm;
  }

  /** A not-signed-on link is the one case a reversal is rejected outright, per docs/03 §7.1. */
  private void respond(ISOSource source, ISOMsg request, String responseCode) {
    try {
      ISOMsg response = (ISOMsg) request.clone();
      response.setResponseMTI();
      response.set(39, responseCode);
      source.send(signed(response));
    } catch (ISOException | java.io.IOException e) {
      warn("failed to send RC " + responseCode + " response", e);
    }
  }

  /**
   * Advices are never declined (docs/03 §7.3), so even an unexpected accept-time failure answers a
   * 0430 - with 96, because nothing was recorded and the acquirer's SAF must repeat it (only "00"
   * completes an advice), as {@code RespondReversal.abort} does.
   */
  private void respondAlways0430(ISOSource source, ISOMsg request) {
    try {
      ISOMsg response = (ISOMsg) request.clone();
      response.setResponseMTI();
      response.set(39, "96");
      source.send(signed(response));
    } catch (ISOException | java.io.IOException e) {
      warn("failed to send 0430 response", e);
    }
  }

  /**
   * MACs the 0430 under the active ZAK. When signing itself fails (the key store is unreachable),
   * the 0430 still goes out unsigned: the gateway only reads its DE 39, and no answer at all would
   * leave the advice hanging until the SAF timeout.
   */
  private ISOMsg signed(ISOMsg response) {
    try {
      return responseMac.sign(response);
    } catch (Exception e) {
      warn("could not MAC 0430, sending it unsigned", e);
      response.unset(64);
      response.unset(128);
      return response;
    }
  }
}
