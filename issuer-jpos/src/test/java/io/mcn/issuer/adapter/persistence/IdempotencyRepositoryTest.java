package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import javax.sql.DataSource;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class IdempotencyRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void storesAndFindsByKeyAndRoute() throws Exception {
    DataSource ds = TestDataSources.migrated(postgres);
    var repo = new IdempotencyRepository(ds);

    try (var conn = ds.getConnection()) {
      repo.store(
          conn, "key-1", "POST /v1/cards/crd_1/blocks", "hash-1", 200, "{\"status\":\"ok\"}");
    }

    var found = repo.find("key-1", "POST /v1/cards/crd_1/blocks");
    assertThat(found).isPresent();
    assertThat(found.get().status()).isEqualTo(200);
    assertThat(found.get().requestHash()).isEqualTo("hash-1");
    assertThat(found.get().body()).contains("ok");
  }

  @Test
  void returnsEmptyWhenNotFound() {
    DataSource ds = TestDataSources.migrated(postgres);
    var repo = new IdempotencyRepository(ds);
    assertThat(repo.find("missing", "POST /v1/cards/crd_1/blocks")).isEmpty();
  }
}
