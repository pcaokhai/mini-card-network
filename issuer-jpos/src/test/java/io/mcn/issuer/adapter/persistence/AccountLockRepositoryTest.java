package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import java.sql.Connection;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class AccountLockRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void locksReadsAndDebitsWithinOneTransaction() throws Exception {
    var ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    long accountId = accounts.insert("ACC-TEST-1", "704", 100_000L);
    var repo = new AccountLockRepository(ds);

    try (Connection conn = ds.getConnection()) {
      conn.setAutoCommit(false);
      var account = repo.lockAndGet(conn, accountId);
      assertThat(account.availableBalance()).isEqualTo(100_000L);

      repo.adjust(conn, accountId, -10_000L);
      conn.commit();
    }

    try (Connection conn = ds.getConnection()) {
      var after = repo.lockAndGet(conn, accountId);
      assertThat(after.availableBalance()).isEqualTo(90_000L);
      assertThat(after.ledgerBalance()).isEqualTo(90_000L);
      assertThat(after.version()).isEqualTo(1L);
    }
  }

  @Test
  @org.junit.jupiter.api.DisplayName("POS-G17: a credit raises both balances")
  void adjustCreditsBothBalances() throws Exception {
    var ds = TestDataSources.migrated(postgres);
    long accountId = new AccountRepository(ds).insert("ACC-TEST-3", "704", 5_000L);
    var repo = new AccountLockRepository(ds);

    try (Connection conn = ds.getConnection()) {
      repo.adjust(conn, accountId, 2_500L);
      var after = repo.lockAndGet(conn, accountId);
      assertThat(after.availableBalance()).isEqualTo(7_500L);
      assertThat(after.ledgerBalance()).isEqualTo(7_500L);
    }
  }

  @Test
  @org.junit.jupiter.api.DisplayName(
      "R-1: a reversal may overdraw - the floor is Authorize's rule, not the account table's")
  void adjustMayTakeTheBalanceBelowTheOverdraftFloor() throws Exception {
    var ds = TestDataSources.migrated(postgres);
    long accountId = new AccountRepository(ds).insert("ACC-TEST-2", "704", 50_000L);
    var repo = new AccountLockRepository(ds);

    try (Connection conn = ds.getConnection()) {
      var after = repo.adjust(conn, accountId, -50_001L);

      assertThat(after.availableBalance()).isEqualTo(-1L);
      assertThat(after.ledgerBalance()).isEqualTo(-1L);
      assertThat(after.overdraftLimit()).isZero();
    }
  }
}
