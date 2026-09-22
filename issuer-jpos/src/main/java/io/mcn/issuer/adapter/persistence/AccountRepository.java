package io.mcn.issuer.adapter.persistence;

import java.sql.SQLException;
import java.sql.Statement;
import java.util.Optional;
import javax.sql.DataSource;

public class AccountRepository {
  private final DataSource dataSource;

  public AccountRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  public Optional<AccountSummary> findById(long accountId) {
    String sql =
        "SELECT account_no, currency, ledger_balance, available_balance FROM account WHERE id = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, accountId);
      var rs = stmt.executeQuery();
      if (!rs.next()) return Optional.empty();
      return Optional.of(
          new AccountSummary(
              rs.getString("account_no"),
              rs.getString("currency"),
              rs.getLong("ledger_balance"),
              rs.getLong("available_balance")));
    } catch (SQLException e) {
      throw new IllegalStateException("find account by id failed", e);
    }
  }

  public long insert(String accountNo, String currency, long ledgerBalance) {
    String sql =
        """
        INSERT INTO account (account_no, currency, ledger_balance, available_balance)
        VALUES (?, ?, ?, ?)""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql, Statement.RETURN_GENERATED_KEYS)) {
      stmt.setString(1, accountNo);
      stmt.setString(2, currency);
      stmt.setLong(3, ledgerBalance);
      stmt.setLong(4, ledgerBalance);
      stmt.executeUpdate();
      var keys = stmt.getGeneratedKeys();
      keys.next();
      return keys.getLong(1);
    } catch (SQLException e) {
      throw new IllegalStateException("insert account failed", e);
    }
  }
}
