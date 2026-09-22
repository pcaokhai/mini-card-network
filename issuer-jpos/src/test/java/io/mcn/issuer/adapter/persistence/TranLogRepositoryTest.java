package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.LocalDate;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class TranLogRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void insertThenFindByDedupeKey() {
    var ds = TestDataSources.migrated(postgres);
    var repo = new TranLogRepository(ds);
    var row =
        new TranLogRow(
            LocalDate.of(2026, 9, 22),
            "0200",
            "PURCHASE",
            "000000",
            "970499",
            "00000042",
            "GOCPHO000000001",
            "000001",
            "0922120000",
            "12345678",
            100_000L,
            "704",
            null,
            "RECEIVED",
            null,
            null,
            null);

    repo.insert(row);
    var found =
        repo.findByDedupeKey(
            "970499", "00000042", "000001", "0922120000", "0200", LocalDate.of(2026, 9, 22));

    assertThat(found).isPresent();
    assertThat(found.get().rrn()).isEqualTo("12345678");
  }

  @Test
  void dedupeLookupMissesOnDifferentStan() {
    var ds = TestDataSources.migrated(postgres);
    var repo = new TranLogRepository(ds);
    assertThat(
            repo.findByDedupeKey(
                "970499", "00000042", "999999", "0922120000", "0200", LocalDate.of(2026, 9, 22)))
        .isEmpty();
  }
}
