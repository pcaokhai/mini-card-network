package io.mcn.issuer.adapter.iso;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.AcquirerLinkRepository;
import io.mcn.issuer.adapter.persistence.JdbcAcquirerLinkRepository;
import io.mcn.issuer.adapter.txn.TxnContextKeys;
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

  public ReversalListener() {}

  public ReversalListener(AcquirerLinkRepository links, String txnMgrName) {
    this.links = links;
    this.txnMgrName = txnMgrName;
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
      source.send(response);
    } catch (ISOException | java.io.IOException e) {
      warn("failed to send RC " + responseCode + " response", e);
    }
  }

  /** Advices are never declined (docs/03 §7.3) - even an unexpected accept-time failure ACKs. */
  private void respondAlways0430(ISOSource source, ISOMsg request) {
    try {
      ISOMsg response = (ISOMsg) request.clone();
      response.setResponseMTI();
      source.send(response);
    } catch (ISOException | java.io.IOException e) {
      warn("failed to send 0430 response", e);
    }
  }
}
