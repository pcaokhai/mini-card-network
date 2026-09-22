package io.mcn.issuer.adapter.http;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.AccountRepository;
import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.IdempotencyRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import org.flywaydb.core.Flyway;
import org.jpos.q2.QBeanSupport;

/**
 * Q2 bean exposing health endpoints plus the {@code /v1/cards*} Admin API (MCN-308); readiness
 * drops before the server stops (NFR-09).
 */
public final class HttpEndpoints extends QBeanSupport {
  private final Readiness readiness = new Readiness();
  private HikariDataSource dataSource;
  private HealthServer server;

  @Override
  protected void startService() {
    HikariConfig hikariConfig = new HikariConfig();
    hikariConfig.setJdbcUrl(cfg.get("jdbc-url"));
    hikariConfig.setUsername(cfg.get("jdbc-user"));
    hikariConfig.setPassword(cfg.get("jdbc-password"));
    dataSource = new HikariDataSource(hikariConfig);
    Flyway.configure().dataSource(dataSource).load().migrate();

    var cardAdmin =
        new CardAdminController(
            dataSource,
            new CardRepository(dataSource),
            new AccountRepository(dataSource),
            new CardLimitRepository(dataSource),
            new AuditLogRepository(dataSource),
            new IdempotencyRepository(dataSource),
            new LedgerRepository());
    server = new HealthServer(readiness, cardAdmin);
    server.start(cfg.getInt("port", 8081));
  }

  @Override
  protected void stopService() throws InterruptedException {
    readiness.startDraining();
    Thread.sleep(cfg.getLong("drain-ms", 2000));
    server.stop();
    if (dataSource != null) {
      dataSource.close();
    }
  }
}
