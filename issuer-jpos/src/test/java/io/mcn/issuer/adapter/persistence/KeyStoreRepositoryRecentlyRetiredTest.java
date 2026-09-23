package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import java.util.Optional;
import javax.sql.DataSource;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class KeyStoreRepositoryRecentlyRetiredTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void should_find_recently_retired_key_within_window__MCN_504_AC2() {
    DataSource ds = TestDataSources.migrated(postgres);
    KeyStoreRepository repo = new KeyStoreRepository(ds);

    long firstId =
        repo.insert(
            new KeyStoreRow(
                0, "ZAK", "970436000", "aa".repeat(16), "AAAAAA", "PENDING", null, null, null));
    repo.activate(firstId);
    long secondId =
        repo.insert(
            new KeyStoreRow(
                0, "ZAK", "970436000", "bb".repeat(16), "BBBBBB", "PENDING", null, null, null));
    repo.activate(secondId); // retires firstId

    Optional<KeyStoreRow> recent =
        repo.findRecentlyRetired("ZAK", "970436000", Duration.ofMinutes(5));
    assertThat(recent).isPresent();
    assertThat(recent.get().id()).isEqualTo(firstId);

    Optional<KeyStoreRow> stale = repo.findRecentlyRetired("ZAK", "970436000", Duration.ZERO);
    assertThat(stale).isEmpty();
  }
}
