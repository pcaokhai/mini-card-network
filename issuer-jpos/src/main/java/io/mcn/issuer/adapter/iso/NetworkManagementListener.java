package io.mcn.issuer.adapter.iso;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.AcquirerLinkRepository;
import io.mcn.issuer.adapter.persistence.JdbcAcquirerLinkRepository;
import org.flywaydb.core.Flyway;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.ISORequestListener;
import org.jpos.iso.ISOSource;
import org.jpos.util.Log;

/**
 * ISORequestListener entry point wired via the {@code <request-listener>} element in the Q2 server
 * deploy descriptor. Constructor injection for tests; {@link #setConfiguration} builds the
 * DataSource from Q2 properties for the real deployment (ponytail: no separate datasource QBean or
 * registry for a single consumer — add one if a second QBean needs the same DataSource).
 */
public final class NetworkManagementListener extends Log
    implements ISORequestListener, Configurable {
  private AcquirerLinkRepository links;

  public NetworkManagementListener() {}

  public NetworkManagementListener(AcquirerLinkRepository links) {
    this.links = links;
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
  }

  @Override
  public boolean process(ISOSource source, ISOMsg request) {
    try {
      ISOMsg response = new HandleNetworkManagement(links).handle(request);
      if (response == null) {
        return false; // not ours (e.g. a financial request) - let the next listener handle it
      }
      source.send(response);
    } catch (Exception e) {
      warn("failed to handle request, responding RC 96", e);
      respondSystemMalfunction(source, request);
    }
    return true;
  }

  private void respondSystemMalfunction(ISOSource source, ISOMsg request) {
    try {
      ISOMsg response = (ISOMsg) request.clone();
      response.setResponseMTI();
      response.set(39, "96");
      source.send(response);
    } catch (ISOException | java.io.IOException e) {
      warn("failed to send RC 96 fallback response", e);
    }
  }
}
