package io.mcn.issuer.adapter.persistence;

import java.sql.SQLException;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.List;
import javax.sql.DataSource;

/**
 * Reads {@code card_limit} (ceilings) and {@code velocity_counter} (today's usage). MCN-301
 * didn't need these; added here rather than growing scope back into that story since CheckLimits
 * is this story's own scope.
 */
public class CardLimitRepository {
  private final DataSource dataSource;

  public CardLimitRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  public List<CardLimit> findApplicableLimits(long cardId, String tranType) {
    String sql =
        "SELECT tran_type, period, max_amount, max_count FROM card_limit "
            + "WHERE card_id = ? AND tran_type IN (?, 'ALL')";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, cardId);
      stmt.setString(2, tranType);
      var rs = stmt.executeQuery();
      List<CardLimit> limits = new ArrayList<>();
      while (rs.next()) {
        long rawMaxAmount = rs.getLong("max_amount");
        Long maxAmount = rs.wasNull() ? null : rawMaxAmount;
        int rawMaxCount = rs.getInt("max_count");
        Integer maxCount = rs.wasNull() ? null : rawMaxCount;
        limits.add(new CardLimit(rs.getString("tran_type"), rs.getString("period"), maxAmount, maxCount));
      }
      return limits;
    } catch (SQLException e) {
      throw new IllegalStateException("find card_limit failed", e);
    }
  }

  public int countToday(long cardId, String tranType) {
    String sql =
        "SELECT txn_count FROM velocity_counter "
            + "WHERE card_id = ? AND tran_type = ? AND period = 'DAILY' AND period_key = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, cardId);
      stmt.setString(2, tranType);
      stmt.setString(3, LocalDate.now().toString());
      var rs = stmt.executeQuery();
      if (!rs.next()) return 0;
      return rs.getInt("txn_count");
    } catch (SQLException e) {
      throw new IllegalStateException("find velocity_counter failed", e);
    }
  }
}
