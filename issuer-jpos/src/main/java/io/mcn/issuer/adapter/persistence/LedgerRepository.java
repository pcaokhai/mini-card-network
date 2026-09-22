package io.mcn.issuer.adapter.persistence;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.time.LocalDate;

/**
 * Posts a balanced double-entry journal for a purchase: one {@code journal_entry} plus two {@code
 * ledger_posting} rows (customer account debit, {@code SETTLEMENT_SUSPENSE} credit). Takes the
 * caller's {@link Connection} (never its own) - the same connection/transaction the row lock and
 * debit ran in, because {@code trg_journal_balanced} is a deferred constraint trigger that only
 * fires at COMMIT of that one transaction.
 */
public class LedgerRepository {

  private static final String SETTLEMENT_SUSPENSE_GL_CODE = "SETTLEMENT_SUSPENSE";

  public LedgerRepository() {}

  public long postPurchase(
      Connection conn,
      long tranId,
      LocalDate businessDate,
      long accountId,
      long amount,
      String currency) {
    try {
      long journalId = insertJournalEntry(conn, tranId, businessDate);
      insertPosting(conn, journalId, accountId, null, "D", amount, currency);
      insertPosting(conn, journalId, null, SETTLEMENT_SUSPENSE_GL_CODE, "C", amount, currency);
      return journalId;
    } catch (SQLException e) {
      throw new IllegalStateException("post purchase journal failed", e);
    }
  }

  private long insertJournalEntry(Connection conn, long tranId, LocalDate businessDate)
      throws SQLException {
    String sql =
        """
        INSERT INTO journal_entry (tran_id, tran_business_date, entry_type)
        VALUES (?, ?, 'PURCHASE')""";
    try (PreparedStatement stmt = conn.prepareStatement(sql, Statement.RETURN_GENERATED_KEYS)) {
      stmt.setLong(1, tranId);
      stmt.setObject(2, businessDate);
      stmt.executeUpdate();
      try (ResultSet keys = stmt.getGeneratedKeys()) {
        keys.next();
        return keys.getLong(1);
      }
    }
  }

  private void insertPosting(
      Connection conn,
      long journalId,
      Long accountId,
      String glCode,
      String direction,
      long amount,
      String currency)
      throws SQLException {
    String sql =
        """
        INSERT INTO ledger_posting (journal_id, account_id, gl_code, direction, amount, currency)
        VALUES (?, ?, ?, ?, ?, ?)""";
    try (PreparedStatement stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, journalId);
      if (accountId != null) {
        stmt.setLong(2, accountId);
      } else {
        stmt.setNull(2, java.sql.Types.BIGINT);
      }
      stmt.setString(3, glCode);
      stmt.setString(4, direction);
      stmt.setLong(5, amount);
      stmt.setString(6, currency);
      stmt.executeUpdate();
    }
  }
}
