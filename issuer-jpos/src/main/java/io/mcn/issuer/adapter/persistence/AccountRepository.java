package io.mcn.issuer.adapter.persistence;

import java.sql.SQLException;
import java.sql.Statement;
import javax.sql.DataSource;

public class AccountRepository {
  private final DataSource dataSource;

  public AccountRepository(DataSource dataSource) {
    this.dataSource = dataSource;
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
