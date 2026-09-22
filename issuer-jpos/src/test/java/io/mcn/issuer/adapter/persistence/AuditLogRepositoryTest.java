package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import javax.sql.DataSource;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class AuditLogRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void recordsAnAuditEntryWithBeforeAndAfterState() throws Exception {
    DataSource ds = TestDataSources.migrated(postgres);
    var repo = new AuditLogRepository(ds);

    try (var conn = ds.getConnection()) {
      repo.record(
          conn,
          "operator@lab",
          "CARD_BLOCKED",
          "card",
          "crd_abc123",
          "{\"status\":\"ACTIVE\"}",
          "{\"status\":\"BLOCKED\"}");
    }

    var entries = repo.findByEntity("card", "crd_abc123");
    assertThat(entries).hasSize(1);
    assertThat(entries.get(0).action()).isEqualTo("CARD_BLOCKED");
    assertThat(entries.get(0).afterState()).contains("BLOCKED");
  }

  @Test
  void appendOnlyTriggerRejectsUpdateAndDelete() throws Exception {
    DataSource ds = TestDataSources.migrated(postgres);
    var repo = new AuditLogRepository(ds);
    long id;
    try (var conn = ds.getConnection()) {
      id = repo.record(conn, "operator@lab", "CARD_BLOCKED", "card", "crd_immutable", null, null);
    }

    try (var conn = ds.getConnection();
        var stmt = conn.prepareStatement("UPDATE audit_log SET actor = 'x' WHERE id = ?")) {
      stmt.setLong(1, id);
      assertThat(
              org.junit.jupiter.api.Assertions.assertThrows(
                      java.sql.SQLException.class, stmt::executeUpdate)
                  .getMessage())
          .contains("append-only");
    }
  }
}
