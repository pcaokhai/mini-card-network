package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import javax.sql.DataSource;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class CardLimitRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void upsertsThenOverwritesAllLimits() throws Exception {
    DataSource ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    var cards = new CardRepository(ds);
    long accountId = accounts.insert("ACC-LIMIT-1", "704", 1_000_000L);
    long cardId =
        cards.insert(
            accountId, "enc".getBytes(), "hash1".getBytes(), "970436", "0001", "2811", "ACTIVE");
    var repo = new CardLimitRepository(ds);

    try (var conn = ds.getConnection()) {
      repo.upsertAllLimits(conn, cardId, 500_000L, 2_000_000L, 10);
    }
    var limits = repo.findAllForCard(cardId);
    assertThat(limits).hasSize(2);

    try (var conn = ds.getConnection()) {
      repo.upsertAllLimits(conn, cardId, 600_000L, 2_500_000L, 5);
    }
    var updated = repo.findAllForCard(cardId);
    assertThat(updated).hasSize(2);
    assertThat(updated)
        .anySatisfy(
            l -> {
              if (l.period().equals("DAILY")) {
                assertThat(l.maxAmount()).isEqualTo(2_500_000L);
                assertThat(l.maxCount()).isEqualTo(5);
              }
            });
  }
}
