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

      boolean debited = repo.debit(conn, accountId, 10_000L, account.version());
      assertThat(debited).isTrue();
      conn.commit();
    }

    try (Connection conn = ds.getConnection()) {
      var after = repo.lockAndGet(conn, accountId);
      assertThat(after.availableBalance()).isEqualTo(90_000L);
      assertThat(after.version()).isEqualTo(1L);
    }
  }

  @Test
  void debitFailsWhenVersionIsStale() throws Exception {
    var ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    long accountId = accounts.insert("ACC-TEST-2", "704", 50_000L);
    var repo = new AccountLockRepository(ds);

    try (Connection conn = ds.getConnection()) {
      boolean debited = repo.debit(conn, accountId, 1_000L, 999L);
      assertThat(debited).isFalse();
    }
  }
}
