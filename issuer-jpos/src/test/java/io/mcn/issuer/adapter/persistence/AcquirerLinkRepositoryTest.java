package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import javax.sql.DataSource;
import org.flywaydb.core.Flyway;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class AcquirerLinkRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine");

  private DataSource dataSource() {
    HikariConfig cfg = new HikariConfig();
    cfg.setJdbcUrl(postgres.getJdbcUrl());
    cfg.setUsername(postgres.getUsername());
    cfg.setPassword(postgres.getPassword());
    DataSource ds = new HikariDataSource(cfg);
    Flyway.configure().dataSource(ds).load().migrate();
    return ds;
  }

  @Test
  void should_upsert_status_and_read_it_back__MCN_201_AC2() {
    AcquirerLinkRepository repo = new JdbcAcquirerLinkRepository(dataSource());
    repo.upsertStatus("970499", "SIGNED_ON");
    assertThat(repo.findStatus("970499")).contains("SIGNED_ON");
  }

  @Test
  void should_default_to_disconnected_from_seed_row__MCN_201_AC2() {
    AcquirerLinkRepository repo = new JdbcAcquirerLinkRepository(dataSource());
    assertThat(repo.findStatus("970499")).contains("DISCONNECTED");
  }
}
