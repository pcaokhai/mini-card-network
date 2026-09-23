package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.LocalDate;
import javax.sql.DataSource;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class VelocityCounterRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void should_increment_atomically_and_read_back_todays_count_and_amount__MCN_803_AC1()
      throws Exception {
    DataSource ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    var cards = new CardRepository(ds);
    long accountId = accounts.insert("ACC-VEL-1", "704", 1_000_000L);
    long cardId =
        cards.insert(
            accountId, "enc".getBytes(), "hash-vel-1".getBytes(), "970436", "0001", "2811",
            "ACTIVE");
    var repo = new VelocityCounterRepository(ds);
    LocalDate today = LocalDate.now();

    repo.incrementDaily(cardId, "PURCHASE", today, 5_000L);
    repo.incrementDaily(cardId, "PURCHASE", today, 3_000L);

    VelocityCounterRow row = repo.findDaily(cardId, "PURCHASE", today).orElseThrow();
    assertThat(row.txnCount()).isEqualTo(2);
    assertThat(row.txnAmount()).isEqualTo(8_000L);
  }

  @Test
  void should_start_empty_when_no_row_exists_yet__MCN_803_AC1() throws Exception {
    DataSource ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    var cards = new CardRepository(ds);
    long accountId = accounts.insert("ACC-VEL-2", "704", 1_000_000L);
    long cardId =
        cards.insert(
            accountId, "enc".getBytes(), "hash-vel-2".getBytes(), "970436", "0002", "2811",
            "ACTIVE");
    var repo = new VelocityCounterRepository(ds);

    assertThat(repo.findDaily(cardId, "PURCHASE", LocalDate.now())).isEmpty();
  }
}
