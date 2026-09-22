package io.mcn.issuer.adapter.persistence;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.Optional;
import javax.sql.DataSource;
import org.postgresql.util.PGobject;

/**
 * Backs {@code Idempotency-Key} replay/mismatch handling (docs/04 §2) on {@code
 * idempotency_record}, keyed by (key, route). {@link #store} takes the caller's {@link
 * Connection} so the record commits in the same transaction as the state change it replays.
 */
public class IdempotencyRepository {
  private final DataSource dataSource;

  public IdempotencyRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  public Optional<IdempotencyRecord> find(String key, String route) {
    String sql =
        "SELECT key, route, request_hash, status, body FROM idempotency_record "
            + "WHERE key = ? AND route = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, key);
      stmt.setString(2, route);
      try (ResultSet rs = stmt.executeQuery()) {
        if (!rs.next()) return Optional.empty();
        return Optional.of(
            new IdempotencyRecord(
                rs.getString("key"),
                rs.getString("route"),
                rs.getString("request_hash"),
                rs.getInt("status"),
                rs.getString("body")));
      }
    } catch (SQLException e) {
      throw new IllegalStateException("find idempotency_record failed", e);
    }
  }

  public void store(
      Connection conn, String key, String route, String requestHash, int status, String bodyJson) {
    String sql =
        """
        INSERT INTO idempotency_record (key, route, request_hash, status, body)
        VALUES (?, ?, ?, ?, ?)""";
    try (PreparedStatement stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, key);
      stmt.setString(2, route);
      stmt.setString(3, requestHash);
      stmt.setInt(4, status);
      PGobject body = new PGobject();
      body.setType("jsonb");
      body.setValue(bodyJson);
      stmt.setObject(5, body);
      stmt.executeUpdate();
    } catch (SQLException e) {
      throw new IllegalStateException("store idempotency_record failed", e);
    }
  }
}
