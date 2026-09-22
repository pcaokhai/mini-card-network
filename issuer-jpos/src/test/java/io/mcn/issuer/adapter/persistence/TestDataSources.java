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
    DataSource ds = new HikariDataSource(cfg);
    Flyway.configure().dataSource(ds).load().migrate();
    return ds;
  }
}
