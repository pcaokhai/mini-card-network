package io.mcn.issuer.adapter.persistence;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import javax.sql.DataSource;
import org.flywaydb.core.Flyway;
import org.testcontainers.containers.PostgreSQLContainer;

/** Shared Testcontainers-Postgres-plus-Flyway-migrate helper for repository/schema tests. */
public final class TestDataSources {
  private TestDataSources() {}

  public static DataSource migrated(PostgreSQLContainer<?> postgres) {
    HikariConfig cfg = new HikariConfig();
    cfg.setJdbcUrl(postgres.getJdbcUrl());
    cfg.setUsername(postgres.getUsername());
    cfg.setPassword(postgres.getPassword());
    // Hikari's default pool (10) is smaller than ConcurrentPurchaseLoadTest's 50-thread client
    // pool: every caller serializes on the account's row lock anyway, but with too few
    // connections most threads also queue for a *connection* before they even reach that lock,
    // stacking Hikari's connectionTimeout wait on top of real DB latency and blowing the test's
    // p99 budget on busier hardware (e.g. CI) even though the ledger invariants stay correct.
    cfg.setMaximumPoolSize(50);
    DataSource ds = new HikariDataSource(cfg);
    Flyway.configure().dataSource(ds).load().migrate();
    return ds;
  }
}
