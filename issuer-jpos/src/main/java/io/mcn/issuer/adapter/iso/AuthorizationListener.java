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
 * {@code ISORequestListener} for financial (MTI class 01/02) requests, wired alongside {@link
 * NetworkManagementListener} in {@code 30_iso_server.xml}. Rejects a request on a link that isn't
 * signed on with RC 91 directly (docs/03 §7.1) rather than queueing it; otherwise hands the message
 * to the {@code TransactionManager} named by the {@code txn-mgr-name} property and returns
 * immediately - the response is sent later, from within the chain, by {@code Respond}.
 */
public final class AuthorizationListener extends Log implements ISORequestListener, Configurable {

  private static final String LAB_ACQUIRER_ID = "970499"; // docs/03 §3

  private AcquirerLinkRepository links;
  private String txnMgrName;

  public AuthorizationListener() {}

  public AuthorizationListener(AcquirerLinkRepository links, String txnMgrName) {
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
    this.txnMgrName = cfg.get("txn-mgr-name", "iso-txn-mgr");
  }

  @Override
  public boolean process(ISOSource source, ISOMsg request) {
    try {
      String mtiClass = request.getMTI().substring(0, 2);
      if (!"01".equals(mtiClass) && !"02".equals(mtiClass)) {
        return false; // not ours - let NetworkManagementListener (or another listener) handle it
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
      warn("failed to accept authorization request, responding RC 96", e);
      respond(source, request, "96");
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
}
