package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import javax.sql.DataSource;
import org.flywaydb.core.Flyway;
import org.jpos.core.Configuration;

/**
 * Builds the {@link DataSource} every participant in this chain needs from Q2's {@code jdbc-url}
 * /{@code jdbc-user}/{@code jdbc-password} properties (same shape as {@code
 * NetworkManagementListener}, MCN-201's precedent for a single-consumer datasource with no shared
 * registry).
 */
final class TxnDataSource {
  private TxnDataSource() {}

  static DataSource fromConfig(Configuration cfg) {
    HikariConfig hikariConfig = new HikariConfig();
    hikariConfig.setJdbcUrl(cfg.get("jdbc-url"));
    hikariConfig.setUsername(cfg.get("jdbc-user"));
    hikariConfig.setPassword(cfg.get("jdbc-password"));
    HikariDataSource dataSource = new HikariDataSource(hikariConfig);
    Flyway.configure().dataSource(dataSource).load().migrate();
    return dataSource;
  }
}
