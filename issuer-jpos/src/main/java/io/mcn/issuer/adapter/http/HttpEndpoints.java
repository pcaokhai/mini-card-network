package io.mcn.issuer.adapter.http;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import io.mcn.issuer.adapter.persistence.AccountRepository;
import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.IdempotencyRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import org.flywaydb.core.Flyway;
import org.jpos.q2.QBeanSupport;

/**
 * Q2 bean exposing health endpoints plus the {@code /v1/cards*} Admin API (MCN-308) and {@code
 * /v1/keys/issuer} (MCN-501); readiness drops before the server stops (NFR-09). The LMK test
 * value is read eagerly here so a missing/blank env var fails Q2 startup (root CLAUDE.md §6
 * rule 11), same fail-fast shape as {@code CheckCard}'s {@code CardCrypto} wiring.
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

    // fail-fast per root CLAUDE.md §6 rule 11; constructor validates non-blank.
    new JCESecurityModule(env("LMK_TEST_VALUE_HEX"));

    var cardAdmin =
        new CardAdminController(
            dataSource,
            new CardRepository(dataSource),
            new AccountRepository(dataSource),
            new CardLimitRepository(dataSource),
            new AuditLogRepository(dataSource),
            new IdempotencyRepository(dataSource),
            new LedgerRepository());
    var keyStore = new KeyStoreRepository(dataSource);
    var keys = new KeysController(keyStore::findAll);
    server = new HealthServer(readiness, cardAdmin, keys);
    server.start(cfg.getInt("port", 8081));
  }

  private static String env(String name) {
    String value = System.getenv(name);
    return value != null ? value : System.getProperty(name);
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
