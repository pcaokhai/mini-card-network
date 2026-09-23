package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.util.List;
import javax.sql.DataSource;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class KeyStoreRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void should_insert_pending_then_activate_retiring_previous_active__MCN_501_AC1() {
    DataSource ds = TestDataSources.migrated(postgres);
    KeyStoreRepository repo = new KeyStoreRepository(ds);

    long firstId =
        repo.insert(
            new KeyStoreRow(0, "ZPK", "970436000", "aa".repeat(16), "AABBCC", "PENDING", null, null,
                null));
    repo.activate(firstId);

    long secondId =
        repo.insert(
            new KeyStoreRow(0, "ZPK", "970436000", "bb".repeat(16), "DDEEFF", "PENDING", null, null,
                null));
    repo.activate(secondId);

    List<KeyStoreRow> all = repo.findAll();
    assertThat(all).filteredOn(r -> r.id() == firstId).extracting(KeyStoreRow::status)
        .containsExactly("RETIRED");
    assertThat(all).filteredOn(r -> r.id() == secondId).extracting(KeyStoreRow::status)
        .containsExactly("ACTIVE");
  }
}
