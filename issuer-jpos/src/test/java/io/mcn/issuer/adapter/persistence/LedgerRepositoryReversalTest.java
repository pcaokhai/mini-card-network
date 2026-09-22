package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class LedgerRepositoryReversalTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void postReversal_creditsAccountDebitsSuspense_MCN_402_AC1() throws Exception {
    var ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    long accountId = accounts.insert("ACC-REV-1", "704", 0L);
    long tranId = seedTranLog(ds, LocalDate.now());

    var repo = new LedgerRepository();
    record Posting(Long accountId, String glCode, String direction, long amount) {}
    List<Posting> postings = new ArrayList<>();
    long journalId;
    try (Connection conn = ds.getConnection()) {
      conn.setAutoCommit(false);
      journalId = repo.postReversal(conn, tranId, LocalDate.now(), accountId, 10_000L, "704");
      conn.commit();
    }
    try (Connection conn = ds.getConnection();
        PreparedStatement stmt =
            conn.prepareStatement(
                "SELECT account_id, gl_code, direction, amount FROM ledger_posting"
                    + " WHERE journal_id = ?")) {
      stmt.setLong(1, journalId);
      try (ResultSet rs = stmt.executeQuery()) {
        while (rs.next()) {
          long rawAccountId = rs.getLong("account_id");
          postings.add(
              new Posting(
                  rs.wasNull() ? null : rawAccountId,
                  rs.getString("gl_code"),
                  rs.getString("direction"),
                  rs.getLong("amount")));
        }
      }
    }

    assertThat(postings)
        .containsExactlyInAnyOrder(
            new Posting(accountId, null, "C", 10_000L),
            new Posting(null, "SETTLEMENT_SUSPENSE", "D", 10_000L));
  }

  private static long seedTranLog(javax.sql.DataSource ds, LocalDate businessDate) {
    var repo = new TranLogRepository(ds);
    return repo.insert(
        new TranLogRow(
            businessDate,
            "0200",
            "PURCHASE",
            "000000",
            "970499",
            "00000042",
            "GOCPHO000000001",
            "000777",
            "0922140000",
            "RRN000000777",
            10_000L,
            "704",
            null,
            "APPROVED",
            "00",
            "123456",
            null));
  }
}
