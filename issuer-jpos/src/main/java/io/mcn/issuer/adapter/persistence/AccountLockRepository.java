package io.mcn.issuer.adapter.persistence;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import javax.sql.DataSource;

/**
 * Row-locked read and optimistic-versioned debit of {@code account}. Unlike the other simpler
 * repositories in this package, this one never opens or commits its own transaction - the row
 * lock's whole point is to be held across the debit and the ledger journal write, which must share
 * the caller's connection/transaction (MCN-302b's Ruling on jPOS's per-participant connections, see
 * {@code Authorize}'s class javadoc).
 */
public class AccountLockRepository {
  private final DataSource dataSource;

  public AccountLockRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  public AccountRow lockAndGet(Connection conn, long accountId) {
    String sql =
        """
        SELECT id, available_balance, ledger_balance, overdraft_limit, version
        FROM account WHERE id = ? FOR UPDATE""";
    try (PreparedStatement stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, accountId);
      try (ResultSet rs = stmt.executeQuery()) {
        if (!rs.next()) {
          throw new IllegalStateException("account not found: " + accountId);
        }
        return new AccountRow(
            rs.getLong("id"),
            rs.getLong("available_balance"),
            rs.getLong("ledger_balance"),
            rs.getLong("overdraft_limit"),
            rs.getLong("version"));
      }
    } catch (SQLException e) {
      throw new IllegalStateException("lock account failed", e);
    }
  }

  /**
   * Adds {@code signedAmount} (negative = debit) to both balances, in the caller's transaction. The
   * UPDATE row-locks the account itself, so no prior {@link #lockAndGet} is needed. Used for
   * movements that can't be declined for funds (a refund credit, a reversal), unlike {@link
   * #debit}: {@code chk_available_floor} still guards the overdraft floor.
   */
  public AccountRow adjust(Connection conn, long accountId, long signedAmount) {
    String sql =
        """
        UPDATE account SET available_balance = available_balance + ?,
                            ledger_balance = ledger_balance + ?,
                            version = version + 1,
                            updated_at = now()
        WHERE id = ?
        RETURNING id, available_balance, ledger_balance, overdraft_limit, version""";
    try (PreparedStatement stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, signedAmount);
      stmt.setLong(2, signedAmount);
      stmt.setLong(3, accountId);
      try (ResultSet rs = stmt.executeQuery()) {
        if (!rs.next()) {
          throw new IllegalStateException("account not found: " + accountId);
        }
        return new AccountRow(
            rs.getLong("id"),
            rs.getLong("available_balance"),
            rs.getLong("ledger_balance"),
            rs.getLong("overdraft_limit"),
            rs.getLong("version"));
      }
    } catch (SQLException e) {
      throw new IllegalStateException("adjust account balance failed", e);
    }
  }

  /** Exposed so callers (e.g. {@code Authorize}) can open a connection from the same pool. */
  public DataSource dataSource() {
    return dataSource;
  }
}
