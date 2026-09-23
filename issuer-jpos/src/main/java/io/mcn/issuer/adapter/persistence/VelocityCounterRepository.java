package io.mcn.issuer.adapter.persistence;

import java.sql.SQLException;
import java.time.LocalDate;
import java.util.Optional;
import javax.sql.DataSource;

/**
 * Atomically updates and reads {@code velocity_counter}, the fast daily count/amount counter the
 * schema reserves for velocity checks (faster than a {@code SUM} over {@code tran_log}).
 */
public class VelocityCounterRepository {
  private final DataSource dataSource;

  public VelocityCounterRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  /** Upserts today's row in one round trip - no read-then-write race. */
  public void incrementDaily(long cardId, String tranType, LocalDate businessDate, long amount) {
    String sql =
        """
        INSERT INTO velocity_counter (card_id, tran_type, period, period_key, txn_count, txn_amount)
        VALUES (?, ?, 'DAILY', ?, 1, ?)
        ON CONFLICT (card_id, tran_type, period, period_key)
        DO UPDATE SET txn_count = velocity_counter.txn_count + 1,
                      txn_amount = velocity_counter.txn_amount + EXCLUDED.txn_amount""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, cardId);
      stmt.setString(2, tranType);
      stmt.setString(3, businessDate.toString());
      stmt.setLong(4, amount);
      stmt.executeUpdate();
    } catch (SQLException e) {
      throw new IllegalStateException("increment velocity_counter failed", e);
    }
  }

  public Optional<VelocityCounterRow> findDaily(long cardId, String tranType, LocalDate businessDate) {
    String sql =
        "SELECT txn_count, txn_amount FROM velocity_counter "
            + "WHERE card_id = ? AND tran_type = ? AND period = 'DAILY' AND period_key = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, cardId);
      stmt.setString(2, tranType);
      stmt.setString(3, businessDate.toString());
      var rs = stmt.executeQuery();
      if (!rs.next()) return Optional.empty();
      return Optional.of(
          new VelocityCounterRow(
              cardId,
              tranType,
              "DAILY",
              businessDate.toString(),
              rs.getInt("txn_count"),
              rs.getLong("txn_amount")));
    } catch (SQLException e) {
      throw new IllegalStateException("find velocity_counter failed", e);
    }
  }
}
