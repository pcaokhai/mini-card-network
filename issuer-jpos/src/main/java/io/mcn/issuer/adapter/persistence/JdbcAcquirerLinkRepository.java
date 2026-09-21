package io.mcn.issuer.adapter.persistence;

import java.sql.SQLException;
import java.util.Optional;
import javax.sql.DataSource;

public final class JdbcAcquirerLinkRepository implements AcquirerLinkRepository {
  private final DataSource dataSource;

  public JdbcAcquirerLinkRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  @Override
  public void upsertStatus(String acquirerId, String status) {
    String sql =
        "INSERT INTO acquirer_link (acquirer_id, status, updated_at) VALUES (?, ?, now()) "
            + "ON CONFLICT (acquirer_id) DO UPDATE SET status = excluded.status, updated_at = now(), "
            + "last_sign_on_at = CASE WHEN excluded.status = 'SIGNED_ON' THEN now() ELSE"
            + " acquirer_link.last_sign_on_at END";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, acquirerId);
      stmt.setString(2, status);
      stmt.executeUpdate();
    } catch (SQLException e) {
      throw new IllegalStateException("upsertStatus failed for " + acquirerId, e);
    }
  }

  @Override
  public Optional<String> findStatus(String acquirerId) {
    String sql = "SELECT status FROM acquirer_link WHERE acquirer_id = ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, acquirerId);
      try (var rs = stmt.executeQuery()) {
        return rs.next() ? Optional.of(rs.getString("status")) : Optional.empty();
      }
    } catch (SQLException e) {
      throw new IllegalStateException("findStatus failed for " + acquirerId, e);
    }
  }
}
