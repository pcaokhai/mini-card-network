package io.mcn.issuer;

import static org.assertj.core.api.Assertions.assertThat;

import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.Statement;
import java.util.HashSet;
import java.util.Set;
import javax.sql.DataSource;
import org.flywaydb.core.Flyway;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class SchemaMigrationTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void allMigrationsApplyCleanlyFromEmpty() throws Exception {
    com.zaxxer.hikari.HikariConfig cfg = new com.zaxxer.hikari.HikariConfig();
    cfg.setJdbcUrl(postgres.getJdbcUrl());
    cfg.setUsername(postgres.getUsername());
    cfg.setPassword(postgres.getPassword());
    DataSource ds = new com.zaxxer.hikari.HikariDataSource(cfg);
    Flyway flyway = Flyway.configure().dataSource(ds).load();
    int applied = flyway.migrate().migrationsExecuted;
    assertThat(applied).isGreaterThanOrEqualTo(4); // V1 (MCN-201) + V2 + V3 + V4 (MCN-308)

    try (Connection c = ds.getConnection();
        Statement st = c.createStatement()) {
      ResultSet rs =
          st.executeQuery(
              "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'");
      Set<String> tables = new HashSet<>();
      while (rs.next()) tables.add(rs.getString(1));
      assertThat(tables)
          .contains(
              "account",
              "card",
              "card_limit",
              "velocity_counter",
              "tran_log",
              "auth_hold",
              "gl_account",
              "journal_entry",
              "ledger_posting",
              "key_store",
              "recon_totals",
              "outbox_event",
              "audit_log",
              "response_code",
              "system_state",
              "cutover_log",
              "acquirer_link",
              "idempotency_record");

      ResultSet cols =
          st.executeQuery(
              "SELECT column_name FROM information_schema.columns WHERE table_name = 'card'");
      Set<String> cardColumns = new HashSet<>();
      while (cols.next()) cardColumns.add(cols.getString(1));
      assertThat(cardColumns).contains("card_ref", "holder_name");
    }
  }
}
