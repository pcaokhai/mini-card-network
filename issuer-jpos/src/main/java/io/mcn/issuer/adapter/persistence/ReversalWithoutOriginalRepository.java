package io.mcn.issuer.adapter.persistence;

import java.sql.SQLException;
import java.time.LocalDate;
import javax.sql.DataSource;

/**
 * {@code reversal_without_original} (MCN-402): a reversal that could not locate its original is
 * recorded here so a later-arriving original for the same DE 90 key is declined RC 94 (docs/03
 * §7.3) instead of posted.
 */
public class ReversalWithoutOriginalRepository {
  private final DataSource dataSource;

  public ReversalWithoutOriginalRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  public void insert(
      String originalMti,
      String originalStan,
      String originalDe7,
      String originalAcquirer,
      LocalDate businessDate) {
    String sql =
        """
        INSERT INTO reversal_without_original
          (business_date, original_mti, original_stan, original_de7, original_acquirer)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT (original_mti, original_stan, original_de7, original_acquirer) DO NOTHING""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setObject(1, businessDate);
      stmt.setString(2, originalMti);
      stmt.setString(3, originalStan);
      stmt.setString(4, originalDe7);
      stmt.setString(5, originalAcquirer);
      stmt.executeUpdate();
    } catch (SQLException e) {
      throw new IllegalStateException("insert reversal_without_original failed", e);
    }
  }

  public boolean findByKey(
      String originalMti, String originalStan, String originalDe7, String originalAcquirer) {
    String sql =
        """
        SELECT 1 FROM reversal_without_original
        WHERE original_mti = ? AND original_stan = ? AND original_de7 = ? AND original_acquirer = ?""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, originalMti);
      stmt.setString(2, originalStan);
      stmt.setString(3, originalDe7);
      stmt.setString(4, originalAcquirer);
      var rs = stmt.executeQuery();
      return rs.next();
    } catch (SQLException e) {
      throw new IllegalStateException("find reversal_without_original by key failed", e);
    }
  }
}
