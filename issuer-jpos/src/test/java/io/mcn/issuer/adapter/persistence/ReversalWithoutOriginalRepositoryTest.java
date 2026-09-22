package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.LocalDate;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class ReversalWithoutOriginalRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void insertThenFindByKey_returnsTheRow_MCN_402_AC2() {
    var repo = new ReversalWithoutOriginalRepository(TestDataSources.migrated(postgres));

    repo.insert("0200", "000123", "0922140000", "970499     ", LocalDate.now());

    assertThat(repo.findByKey("0200", "000123", "0922140000", "970499     ")).isTrue();
  }

  @Test
  void findByKey_noRow_returnsFalse_MCN_402_AC2() {
    var repo = new ReversalWithoutOriginalRepository(TestDataSources.migrated(postgres));

    assertThat(repo.findByKey("0200", "999999", "0922140000", "970499     ")).isFalse();
  }
}
