package io.mcn.issuer.adapter.persistence;

import java.sql.Connection;
import java.sql.PreparedStatement;
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

  /** The counter every approved debit (purchase or cash) increments and its reversal undoes. */
  public static final String DEBIT_TRAN_TYPE = "PURCHASE";

  private static final String INCREMENT_SQL =
      """
      INSERT INTO velocity_counter (card_id, tran_type, period, period_key, txn_count, txn_amount)
      VALUES (?, ?, 'DAILY', ?, 1, ?)
      ON CONFLICT (card_id, tran_type, period, period_key)
      DO UPDATE SET txn_count = velocity_counter.txn_count + 1,
                    txn_amount = velocity_counter.txn_amount + EXCLUDED.txn_amount""";

  /** Upserts today's row in one round trip - no read-then-write race. */
  public void incrementDaily(long cardId, String tranType, LocalDate businessDate, long amount) {
    try (var conn = dataSource.getConnection()) {
      incrementDaily(conn, cardId, tranType, businessDate, amount);
    } catch (SQLException e) {
      throw new IllegalStateException("increment velocity_counter failed", e);
    }
  }

  /**
   * Same upsert, on a connection the caller owns - so a decline never increments (Authorize calls
   * this only after its own ledger post, on the same open transaction, before commit).
   */
  public void incrementDaily(
      Connection conn, long cardId, String tranType, LocalDate businessDate, long amount) {
    try (PreparedStatement stmt = conn.prepareStatement(INCREMENT_SQL)) {
      stmt.setLong(1, cardId);
      stmt.setString(2, tranType);
      stmt.setString(3, businessDate.toString());
      stmt.setLong(4, amount);
      stmt.executeUpdate();
    } catch (SQLException e) {
      throw new IllegalStateException("increment velocity_counter failed", e);
    }
  }

  /**
   * Undoes one {@link #incrementDaily} on the caller's connection: a reversed debit no longer
   * counts towards the day's limits (R-3). Floors at zero so a replayed reversal can't go negative.
   */
  public void decrementDaily(
      Connection conn, long cardId, String tranType, LocalDate businessDate, long amount) {
    String sql =
        """
        UPDATE velocity_counter SET txn_count = GREATEST(txn_count - 1, 0),
                                    txn_amount = GREATEST(txn_amount - ?, 0)
        WHERE card_id = ? AND tran_type = ? AND period = 'DAILY' AND period_key = ?""";
    try (PreparedStatement stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, amount);
      stmt.setLong(2, cardId);
      stmt.setString(3, tranType);
      stmt.setString(4, businessDate.toString());
      stmt.executeUpdate();
    } catch (SQLException e) {
      throw new IllegalStateException("decrement velocity_counter failed", e);
    }
  }

  public Optional<VelocityCounterRow> findDaily(
      long cardId, String tranType, LocalDate businessDate) {
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
