package io.mcn.issuer.adapter.persistence;

import java.sql.Connection;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.time.Instant;
import java.time.LocalDate;
import java.util.Optional;
import javax.sql.DataSource;

public class TranLogRepository {
  private final DataSource dataSource;

  public TranLogRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  public long insert(TranLogRow row) {
    String sql =
        """
        INSERT INTO tran_log (business_date, mti, tran_type, processing_code, acquirer_id, tid,
                               mid, stan, transmission_dt_raw, transmission_at, rrn, amount,
                               currency, card_id, status, response_code, auth_code, decline_reason)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql, java.sql.Statement.RETURN_GENERATED_KEYS)) {
      stmt.setObject(1, row.businessDate());
      stmt.setString(2, row.mti());
      stmt.setString(3, row.tranType());
      stmt.setString(4, row.processingCode());
      stmt.setString(5, row.acquirerId());
      stmt.setString(6, row.tid());
      stmt.setString(7, row.mid());
      stmt.setString(8, row.stan());
      stmt.setString(9, row.transmissionDtRaw());
      stmt.setTimestamp(10, Timestamp.from(Instant.now()));
      stmt.setString(11, row.rrn());
      stmt.setLong(12, row.amount());
      stmt.setString(13, row.currency());
      if (row.cardId() != null) {
        stmt.setLong(14, row.cardId());
      } else {
        stmt.setNull(14, java.sql.Types.BIGINT);
      }
      stmt.setString(15, row.status());
      stmt.setString(16, row.responseCode());
      stmt.setString(17, row.authCode());
      stmt.setString(18, row.declineReason());
      stmt.executeUpdate();
      var keys = stmt.getGeneratedKeys();
      keys.next();
      return keys.getLong(1);
    } catch (SQLException e) {
      throw new IllegalStateException("insert tran_log failed", e);
    }
  }

  /**
   * Finalizes a {@code tran_log} row {@link #insert} already created (status {@code RECEIVED}) once
   * the outcome is known - used by {@code LogAndOutbox} when the chain reached far enough to have
   * logged the attempt before the outcome was decided (e.g. by {@code Authorize}).
   */
  public void updateOutcome(
      long tranId,
      LocalDate businessDate,
      String status,
      String responseCode,
      String authCode,
      String declineReason) {
    updateOutcome(tranId, businessDate, status, responseCode, authCode, declineReason, null);
  }

  /** As above, plus the balance a balance inquiry answered, so a duplicate replays it (R-2). */
  public void updateOutcome(
      long tranId,
      LocalDate businessDate,
      String status,
      String responseCode,
      String authCode,
      String declineReason,
      Long balance) {
    try (var conn = dataSource.getConnection()) {
      updateOutcome(
          conn, tranId, businessDate, status, responseCode, authCode, declineReason, balance);
    } catch (SQLException e) {
      throw new IllegalStateException("update tran_log outcome failed", e);
    }
  }

  /**
   * Same update on the caller's connection, so {@code Authorize} commits the outcome in the
   * transaction that moves the money (S1, root CLAUDE.md §6.5). Only a row still {@code RECEIVED}
   * takes an outcome: returns false when it already has one (a reversal abandoned it first, or
   * {@code Authorize} already wrote it), so nothing ever overwrites a decided row.
   */
  public boolean updateOutcome(
      Connection conn,
      long tranId,
      LocalDate businessDate,
      String status,
      String responseCode,
      String authCode,
      String declineReason,
      Long balance) {
    String sql =
        """
        UPDATE tran_log SET status = ?, response_code = ?, auth_code = ?, decline_reason = ?,
                             balance = ?, updated_at = now()
        WHERE id = ? AND business_date = ? AND status = 'RECEIVED'""";
    try (var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, status);
      stmt.setString(2, responseCode);
      stmt.setString(3, authCode);
      stmt.setString(4, declineReason);
      stmt.setObject(5, balance, java.sql.Types.BIGINT);
      stmt.setLong(6, tranId);
      stmt.setObject(7, businessDate);
      return stmt.executeUpdate() == 1;
    } catch (SQLException e) {
      throw new IllegalStateException("update tran_log outcome failed", e);
    }
  }

  public Optional<TranLogRow> findByDedupeKey(
      String acquirerId,
      String tid,
      String stan,
      String transmissionDtRaw,
      String mti,
      LocalDate businessDate) {
    String sql =
        """
        SELECT business_date, mti, tran_type, processing_code, acquirer_id, tid, mid, stan,
               transmission_dt_raw, rrn, amount, currency, card_id, status, response_code,
               auth_code, decline_reason, balance
        FROM tran_log
        WHERE acquirer_id = ? AND tid = ? AND stan = ? AND transmission_dt_raw = ?
          AND mti = ? AND business_date = ?""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, acquirerId);
      stmt.setString(2, tid);
      stmt.setString(3, stan);
      stmt.setString(4, transmissionDtRaw);
      stmt.setString(5, mti);
      stmt.setObject(6, businessDate);
      var rs = stmt.executeQuery();
      if (!rs.next()) return Optional.empty();
      long rawCardId = rs.getLong("card_id");
      Long cardId = rs.wasNull() ? null : rawCardId;
      long rawBalance = rs.getLong("balance");
      Long balance = rs.wasNull() ? null : rawBalance;
      return Optional.of(
          new TranLogRow(
              rs.getObject("business_date", LocalDate.class),
              rs.getString("mti").trim(),
              rs.getString("tran_type"),
              rs.getString("processing_code").trim(),
              rs.getString("acquirer_id"),
              rs.getString("tid").trim(),
              rs.getString("mid"),
              rs.getString("stan").trim(),
              rs.getString("transmission_dt_raw").trim(),
              rs.getString("rrn").trim(),
              rs.getLong("amount"),
              rs.getString("currency").trim(),
              cardId,
              rs.getString("status"),
              rs.getString("response_code") == null ? null : rs.getString("response_code").trim(),
              rs.getString("auth_code") == null ? null : rs.getString("auth_code").trim(),
              rs.getString("decline_reason"),
              balance));
    } catch (SQLException e) {
      throw new IllegalStateException("find tran_log by dedupe key failed", e);
    }
  }

  /**
   * Locates the original transaction a reversal's DE 90 refers to. Unlike {@link #findByDedupeKey},
   * this never filters on {@code tid} (DE 90 carries no TID, docs/03 §7.3) or on a business date (a
   * reversal can arrive on a later business date than its original, per docs/03 §7.3, so filtering
   * by the reversal's own business date would miss the original).
   */
  public Optional<OriginalTransactionRow> findByReversalKey(
      String originalMti, String originalStan, String originalDe7, String originalAcquirer) {
    String sql =
        """
        SELECT id, business_date, card_id, amount, currency, status
        FROM tran_log
        WHERE acquirer_id = ? AND stan = ? AND transmission_dt_raw = ? AND mti = ?
        ORDER BY id DESC LIMIT 1""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, originalAcquirer);
      stmt.setString(2, originalStan);
      stmt.setString(3, originalDe7);
      stmt.setString(4, originalMti);
      var rs = stmt.executeQuery();
      if (!rs.next()) return Optional.empty();
      return Optional.of(
          new OriginalTransactionRow(
              rs.getLong("id"),
              rs.getObject("business_date", LocalDate.class),
              rs.getLong("card_id"),
              rs.getLong("amount"),
              rs.getString("currency").trim(),
              rs.getString("status")));
    } catch (SQLException e) {
      throw new IllegalStateException("find tran_log by reversal key failed", e);
    }
  }

  /** Marks the original transaction's row {@code REVERSED} once its reversing journal is posted. */
  /**
   * Flips an APPROVED row to REVERSED on the caller's connection, so it commits with the reversing
   * journal. The row lock the UPDATE takes serialises a 0420 and its 0421 repeat: only the one that
   * sees APPROVED gets {@code true} and posts.
   */
  /**
   * Abandons an original that never finished: still {@code RECEIVED} after {@code staleAfter}
   * (longer than the acquirer's response timeout, docs/03 §9) and with no journal, so it moved no
   * money. It becomes {@code REVERSED}, the status a reversed original with nothing left to undo
   * has; a late {@code Authorize} for it then finds no {@code RECEIVED} row and rolls back. One
   * conditional statement, so it can't interleave with that approval's own outcome write.
   */
  public boolean markAbandoned(
      Connection conn, long tranId, LocalDate businessDate, java.time.Duration staleAfter)
      throws SQLException {
    String sql =
        """
        UPDATE tran_log t SET status = 'REVERSED',
                              decline_reason = 'abandoned: reversed while still RECEIVED',
                              updated_at = now()
        WHERE t.id = ? AND t.business_date = ? AND t.status = 'RECEIVED'
          AND t.created_at < now() - make_interval(secs => ?)
          AND NOT EXISTS (SELECT 1 FROM journal_entry je
                          WHERE je.tran_id = t.id AND je.tran_business_date = t.business_date)""";
    try (var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, tranId);
      stmt.setObject(2, businessDate);
      stmt.setLong(3, staleAfter.toSeconds());
      return stmt.executeUpdate() == 1;
    }
  }

  public boolean markReversed(Connection conn, long tranId, LocalDate businessDate)
      throws SQLException {
    String sql =
        "UPDATE tran_log SET status = 'REVERSED', updated_at = now()"
            + " WHERE id = ? AND business_date = ? AND status = 'APPROVED'";
    try (var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, tranId);
      stmt.setObject(2, businessDate);
      return stmt.executeUpdate() == 1;
    }
  }
}
