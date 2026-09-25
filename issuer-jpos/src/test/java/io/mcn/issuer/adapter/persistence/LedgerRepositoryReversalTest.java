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
  void postReversalOf_aPurchase_creditsAccountDebitsSuspense_MCN_402_AC1() throws Exception {
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
      repo.postPurchase(conn, tranId, LocalDate.now(), accountId, 10_000L, "704");
      var deltas = repo.postReversalOf(conn, tranId, LocalDate.now());
      conn.commit();
      assertThat(deltas).containsExactly(java.util.Map.entry(accountId, 10_000L));
      journalId = latestReversalJournal(conn);
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

  @Test
  @org.junit.jupiter.api.DisplayName("POS-G17: reversing a refund debits the account back")
  void postReversalOf_aRefund_debitsAccountCreditsSuspense() throws Exception {
    var ds = TestDataSources.migrated(postgres);
    long accountId = new AccountRepository(ds).insert("ACC-REV-2", "704", 0L);
    long tranId = seedTranLog(ds, LocalDate.now());
    var repo = new LedgerRepository();

    try (Connection conn = ds.getConnection()) {
      conn.setAutoCommit(false);
      repo.postRefund(conn, tranId, LocalDate.now(), accountId, 7_000L, "704");
      var deltas = repo.postReversalOf(conn, tranId, LocalDate.now());
      conn.commit();

      assertThat(deltas).containsExactly(java.util.Map.entry(accountId, -7_000L));
    }
  }

  @Test
  @org.junit.jupiter.api.DisplayName("POS-G17: an original with no journal reverses to nothing")
  void postReversalOf_anOriginalThatPostedNothing_writesNoJournal() throws Exception {
    var ds = TestDataSources.migrated(postgres);
    long tranId = seedTranLog(ds, LocalDate.now());

    try (Connection conn = ds.getConnection()) {
      conn.setAutoCommit(false);
      assertThat(new LedgerRepository().postReversalOf(conn, tranId, LocalDate.now())).isEmpty();
      conn.commit();
    }
  }

  private static long latestReversalJournal(Connection conn) throws Exception {
    try (var stmt =
            conn.prepareStatement(
                "SELECT max(id) FROM journal_entry WHERE entry_type = 'REVERSAL'");
        var rs = stmt.executeQuery()) {
      rs.next();
      return rs.getLong(1);
    }
  }

  private static final java.util.concurrent.atomic.AtomicInteger NEXT_STAN =
      new java.util.concurrent.atomic.AtomicInteger(777);

  /** A fresh STAN per call: the tests share one container, and the dedupe key must stay unique. */
  private static long seedTranLog(javax.sql.DataSource ds, LocalDate businessDate) {
    var repo = new TranLogRepository(ds);
    String stan = String.format("%06d", NEXT_STAN.getAndIncrement());
    return repo.insert(
        new TranLogRow(
            businessDate,
            "0200",
            "PURCHASE",
            "000000",
            "970499",
            "00000042",
            "GOCPHO000000001",
            stan,
            "0922140000",
            "RRN000" + stan,
            10_000L,
            "704",
            null,
            "APPROVED",
            "00",
            "123456",
            null));
  }
}
