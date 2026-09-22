package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import org.flywaydb.core.Flyway;
import org.jpos.core.Configuration;

/**
 * Builds the {@link HikariDataSource} every participant in this chain needs from Q2's {@code
 * jdbc-url}/{@code jdbc-user}/{@code jdbc-password} properties (same shape as {@code
 * NetworkManagementListener}, MCN-201's precedent for a single-consumer datasource with no shared
 * registry). Returns the concrete type, not {@code javax.sql.DataSource}, so callers implementing
 * {@code org.jpos.util.Destroyable} can close the pool.
 */
final class TxnDataSource {
  private TxnDataSource() {}

  static HikariDataSource fromConfig(Configuration cfg) {
    HikariConfig hikariConfig = new HikariConfig();
    hikariConfig.setJdbcUrl(cfg.get("jdbc-url"));
    hikariConfig.setUsername(cfg.get("jdbc-user"));
    hikariConfig.setPassword(cfg.get("jdbc-password"));
    // ponytail: small per-participant pool - 5 participants each own a pool (no shared datasource
    // registry exists yet); keeps total connections bounded instead of Hikari's default of 10 each.
    hikariConfig.setMaximumPoolSize(2);
    HikariDataSource dataSource = new HikariDataSource(hikariConfig);
    Flyway.configure().dataSource(dataSource).load().migrate();
    return dataSource;
  }
}
