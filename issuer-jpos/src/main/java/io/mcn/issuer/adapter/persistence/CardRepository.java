package io.mcn.issuer.adapter.persistence;

import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;
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

  private static final String SELECT_COLUMNS =
      "id, account_id, bin, pan_last4, expiry_yymm, status, card_ref, holder_name";

  public Optional<Card> findByPanHash(byte[] panHash) {
    String sql = "SELECT " + SELECT_COLUMNS + " FROM card WHERE pan_hash = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setBytes(1, panHash);
      var rs = stmt.executeQuery();
      if (!rs.next()) return Optional.empty();
      return Optional.of(toCard(rs));
    } catch (SQLException e) {
      throw new IllegalStateException("find card by pan_hash failed", e);
    }
  }

  /** MCN-402: resolves the account a reversal's original card belongs to. */
  public Optional<Card> findById(long cardId) {
    String sql = "SELECT " + SELECT_COLUMNS + " FROM card WHERE id = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, cardId);
      var rs = stmt.executeQuery();
      if (!rs.next()) return Optional.empty();
      return Optional.of(toCard(rs));
    } catch (SQLException e) {
      throw new IllegalStateException("find card by id failed", e);
    }
  }

  public Optional<Card> findByCardRef(String cardRef) {
    String sql = "SELECT " + SELECT_COLUMNS + " FROM card WHERE card_ref = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, cardRef);
      var rs = stmt.executeQuery();
      if (!rs.next()) return Optional.empty();
      return Optional.of(toCard(rs));
    } catch (SQLException e) {
      throw new IllegalStateException("find card by card_ref failed", e);
    }
  }

  public List<Card> findAll() {
    String sql = "SELECT " + SELECT_COLUMNS + " FROM card ORDER BY id";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql);
        var rs = stmt.executeQuery()) {
      List<Card> cards = new ArrayList<>();
      while (rs.next()) cards.add(toCard(rs));
      return cards;
    } catch (SQLException e) {
      throw new IllegalStateException("find all cards failed", e);
    }
  }

  /** Sets {@code status} in the same transaction the caller opened (e.g. block/unblock + audit). */
  public boolean updateStatus(java.sql.Connection conn, long cardId, String status) {
    String sql = "UPDATE card SET status = ?, updated_at = now() WHERE id = ?";
    try (var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, status);
      stmt.setLong(2, cardId);
      return stmt.executeUpdate() == 1;
    } catch (SQLException e) {
      throw new IllegalStateException("update card status failed", e);
    }
  }

  public Optional<String> findPvv(long cardId) {
    String sql = "SELECT pvv FROM card WHERE id = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, cardId);
      var rs = stmt.executeQuery();
      if (!rs.next()) return Optional.empty();
      return Optional.ofNullable(rs.getString(1));
    } catch (SQLException e) {
      throw new IllegalStateException("find card pvv failed", e);
    }
  }

  /** Returns the new {@code pin_try_count} after incrementing (used to decide the third strike). */
  public int incrementPinTryCount(long cardId) {
    String sql =
        "UPDATE card SET pin_try_count = pin_try_count + 1, updated_at = now() WHERE id = ?"
            + " RETURNING pin_try_count";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, cardId);
      var rs = stmt.executeQuery();
      rs.next();
      return rs.getInt(1);
    } catch (SQLException e) {
      throw new IllegalStateException("increment pin_try_count failed", e);
    }
  }

  public void resetPinTryCount(long cardId) {
    String sql = "UPDATE card SET pin_try_count = 0, updated_at = now() WHERE id = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, cardId);
      stmt.executeUpdate();
    } catch (SQLException e) {
      throw new IllegalStateException("reset pin_try_count failed", e);
    }
  }

  public void blockForPin(long cardId) {
    String sql = "UPDATE card SET status = 'PIN_BLOCKED', updated_at = now() WHERE id = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setLong(1, cardId);
      stmt.executeUpdate();
    } catch (SQLException e) {
      throw new IllegalStateException("block card for pin failed", e);
    }
  }

  private Card toCard(java.sql.ResultSet rs) throws SQLException {
    return new Card(
        rs.getLong(1),
        rs.getLong(2),
        rs.getString(3),
        rs.getString(4),
        rs.getString(5),
        rs.getString(6),
        rs.getString(7),
        rs.getString(8));
  }
}
