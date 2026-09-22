package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.sql.Connection;
import java.time.LocalDate;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class LedgerRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void postsABalancedJournalForAPurchase() throws Exception {
    var ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    long accountId = accounts.insert("ACC-LEDGER-1", "704", 100_000L);
    var tranLogRepo = new TranLogRepository(ds);
    long tranId = tranLogRepo.insert(sampleRow());
    var repo = new LedgerRepository();

    try (Connection conn = ds.getConnection()) {
      conn.setAutoCommit(false);
      long journalId = repo.postPurchase(conn, tranId, LocalDate.now(), accountId, 10_000L, "704");
      conn.commit(); // trg_journal_balanced fires here - must not throw
      assertThat(journalId).isPositive();
    }
  }

  private TranLogRow sampleRow() {
    return new TranLogRow(
        LocalDate.now(),
        "0200",
        "PURCHASE",
        "000000",
        "970499",
        "00000042",
        "GOCPHO000000001",
        "000001",
        "0922120000",
        "GOCPHO000000",
        10_000L,
        "704",
        null,
        "RECEIVED",
        null,
        null,
        null);
  }
}
