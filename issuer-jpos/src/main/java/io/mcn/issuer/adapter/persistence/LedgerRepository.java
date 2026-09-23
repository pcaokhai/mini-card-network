package io.mcn.issuer.adapter.persistence;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import javax.sql.DataSource;

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
      long journalId = insertJournalEntry(conn, tranId, businessDate, "PURCHASE");
      insertPosting(conn, journalId, accountId, null, "D", amount, currency);
      insertPosting(conn, journalId, null, SETTLEMENT_SUSPENSE_GL_CODE, "C", amount, currency);
      return journalId;
    } catch (SQLException e) {
      throw new IllegalStateException("post purchase journal failed", e);
    }
  }

  /**
   * Posts a balanced reversing journal: the customer's money comes back (credit the account, debit
   * {@code SETTLEMENT_SUSPENSE}) - the debit/credit sides swapped from {@link #postPurchase}.
   */
  public long postReversal(
      Connection conn,
      long tranId,
      LocalDate businessDate,
      long accountId,
      long amount,
      String currency) {
    try {
      long journalId = insertJournalEntry(conn, tranId, businessDate, "REVERSAL");
      insertPosting(conn, journalId, accountId, null, "C", amount, currency);
      insertPosting(conn, journalId, null, SETTLEMENT_SUSPENSE_GL_CODE, "D", amount, currency);
      return journalId;
    } catch (SQLException e) {
      throw new IllegalStateException("post reversal journal failed", e);
    }
  }

  private long insertJournalEntry(
      Connection conn, long tranId, LocalDate businessDate, String entryType) throws SQLException {
    String sql =
        """
        INSERT INTO journal_entry (tran_id, tran_business_date, entry_type)
        VALUES (?, ?, ?)""";
    try (PreparedStatement stmt = conn.prepareStatement(sql, Statement.RETURN_GENERATED_KEYS)) {
      stmt.setLong(1, tranId);
      stmt.setObject(2, businessDate);
      stmt.setString(3, entryType);
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

  /**
   * Journal entries touching {@code accountId}, newest first, cursor-paginated by journal id
   * (docs/04 §2 cursor convention). Read-only admin-API query (MCN-308) - unlike {@link
   * #postPurchase}, this owns its own connection since it isn't part of a write transaction.
   */
  public List<JournalEntryRow> findByAccount(
      DataSource ds, long accountId, int limit, Long beforeJournalId) {
    try (Connection conn = ds.getConnection()) {
      List<Long> journalIds = journalIdsForAccount(conn, accountId, limit, beforeJournalId);
      if (journalIds.isEmpty()) return List.of();
      return postingsForJournals(conn, journalIds);
    } catch (SQLException e) {
      throw new IllegalStateException("find journal entries by account failed", e);
    }
  }

  private List<Long> journalIdsForAccount(
      Connection conn, long accountId, int limit, Long beforeJournalId) throws SQLException {
    String sql =
        """
        SELECT DISTINCT journal_id FROM ledger_posting jp
        JOIN journal_entry je ON je.id = jp.journal_id
        WHERE jp.account_id = ? AND (? IS NULL OR jp.journal_id < ?)
        ORDER BY journal_id DESC LIMIT ?""";
    try (PreparedStatement stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, accountId);
      if (beforeJournalId != null) {
        stmt.setLong(2, beforeJournalId);
        stmt.setLong(3, beforeJournalId);
      } else {
        stmt.setNull(2, java.sql.Types.BIGINT);
        stmt.setNull(3, java.sql.Types.BIGINT);
      }
      stmt.setInt(4, limit);
      try (ResultSet rs = stmt.executeQuery()) {
        List<Long> ids = new ArrayList<>();
        while (rs.next()) ids.add(rs.getLong(1));
        return ids;
      }
    }
  }

  private List<JournalEntryRow> postingsForJournals(Connection conn, List<Long> journalIds)
      throws SQLException {
    String placeholders = String.join(",", journalIds.stream().map(id -> "?").toList());
    String sql =
        "SELECT je.id AS journal_id, je.entry_type, je.created_at, t.rrn, "
            + "p.gl_code, p.direction, p.amount, p.currency, a.account_no "
            + "FROM journal_entry je "
            + "JOIN ledger_posting p ON p.journal_id = je.id "
            + "LEFT JOIN tran_log t ON t.id = je.tran_id AND t.business_date = je.tran_business_date "
            + "LEFT JOIN account a ON a.id = p.account_id "
            + "WHERE je.id IN ("
            + placeholders
            + ") ORDER BY je.id DESC, p.id ASC";
    try (PreparedStatement stmt = conn.prepareStatement(sql)) {
      for (int i = 0; i < journalIds.size(); i++) stmt.setLong(i + 1, journalIds.get(i));
      try (ResultSet rs = stmt.executeQuery()) {
        Map<Long, JournalEntryRow> byId = new LinkedHashMap<>();
        while (rs.next()) {
          long journalId = rs.getLong("journal_id");
          JournalEntryRow row = byId.get(journalId);
          if (row == null) {
            row =
                new JournalEntryRow(
                    journalId,
                    rs.getString("entry_type"),
                    instantOf(rs, "created_at"),
                    rs.getString("rrn"),
                    new ArrayList<>());
            byId.put(journalId, row);
          }
          String account = rs.getString("gl_code");
          if (account == null) account = rs.getString("account_no");
          row.postings()
              .add(
                  new PostingRow(
                      account,
                      rs.getString("direction"),
                      rs.getLong("amount"),
                      rs.getString("currency")));
        }
        return new ArrayList<>(byId.values());
      }
    }
  }

  private static java.time.Instant instantOf(ResultSet rs, String column) {
    try {
      return rs.getTimestamp(column).toInstant();
    } catch (SQLException e) {
      throw new IllegalStateException("read timestamp failed", e);
    }
  }
}
