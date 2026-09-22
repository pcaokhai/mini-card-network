package io.mcn.issuer.adapter.persistence;

import java.sql.SQLException;
import java.sql.Statement;
import java.util.Optional;
import javax.sql.DataSource;

public class CardRepository {
  private final DataSource dataSource;

  public CardRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  public long insert(
      long accountId,
      byte[] panEnc,
      byte[] panHash,
      String bin,
      String panLast4,
      String expiryYymm,
      String status) {
    String sql =
        """
        INSERT INTO card (account_id, pan_enc, pan_hash, bin, pan_last4, expiry_yymm, status)
        VALUES (?, ?, ?, ?, ?, ?, ?)""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql, Statement.RETURN_GENERATED_KEYS)) {
      stmt.setLong(1, accountId);
      stmt.setBytes(2, panEnc);
      stmt.setBytes(3, panHash);
      stmt.setString(4, bin);
      stmt.setString(5, panLast4);
      stmt.setString(6, expiryYymm);
      stmt.setString(7, status);
      stmt.executeUpdate();
      var keys = stmt.getGeneratedKeys();
      keys.next();
      return keys.getLong(1);
    } catch (SQLException e) {
      throw new IllegalStateException("insert card failed", e);
    }
  }

  public Optional<Card> findByPanHash(byte[] panHash) {
    String sql =
        "SELECT id, account_id, bin, pan_last4, expiry_yymm, status FROM card WHERE pan_hash = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setBytes(1, panHash);
      var rs = stmt.executeQuery();
      if (!rs.next()) return Optional.empty();
      return Optional.of(
          new Card(
              rs.getLong(1),
              rs.getLong(2),
              rs.getString(3),
              rs.getString(4),
              rs.getString(5),
              rs.getString(6)));
    } catch (SQLException e) {
      throw new IllegalStateException("find card by pan_hash failed", e);
    }
  }
}
