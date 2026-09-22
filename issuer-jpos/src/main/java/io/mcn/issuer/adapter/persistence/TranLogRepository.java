package io.mcn.issuer.adapter.persistence;

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
    String sql =
        """
        UPDATE tran_log SET status = ?, response_code = ?, auth_code = ?, decline_reason = ?,
                             updated_at = now()
        WHERE id = ? AND business_date = ?""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, status);
      stmt.setString(2, responseCode);
      stmt.setString(3, authCode);
      stmt.setString(4, declineReason);
      stmt.setLong(5, tranId);
      stmt.setObject(6, businessDate);
      stmt.executeUpdate();
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
               auth_code, decline_reason
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
              rs.getString("decline_reason")));
    } catch (SQLException e) {
      throw new IllegalStateException("find tran_log by dedupe key failed", e);
    }
  }

  /**
   * Locates the original transaction a reversal's DE 90 refers to. Unlike {@link
   * #findByDedupeKey}, this never filters on {@code tid} (DE 90 carries no TID, docs/03 §7.3) or on
   * a business date (a reversal can arrive on a later business date than its original, per docs/03
   * §7.3, so filtering by the reversal's own business date would miss the original).
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
  public void markReversed(long tranId, LocalDate businessDate) {
    String sql =
        "UPDATE tran_log SET status = 'REVERSED', updated_at = now() WHERE id = ? AND business_date = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, tranId);
      stmt.setObject(2, businessDate);
      stmt.executeUpdate();
    } catch (SQLException e) {
      throw new IllegalStateException("mark tran_log reversed failed", e);
    }
  }
}
