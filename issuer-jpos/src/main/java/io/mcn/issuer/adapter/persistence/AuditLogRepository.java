package io.mcn.issuer.adapter.persistence;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;
import javax.sql.DataSource;
import org.postgresql.util.PGobject;

/**
 * Appends to {@code audit_log} (append-only; {@code trg_audit_immutable} rejects any UPDATE or
 * DELETE at the DB level). {@link #record} takes the caller's {@link Connection} so it commits in
 * the same transaction as the state change it is auditing.
 */
public class AuditLogRepository {
  private final DataSource dataSource;

  public AuditLogRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  public long record(
      Connection conn,
      String actor,
      String action,
      String entityType,
      String entityId,
      String beforeStateJson,
      String afterStateJson) {
    String sql =
        """
        INSERT INTO audit_log (actor, action, entity_type, entity_id, before_state, after_state)
        VALUES (?, ?, ?, ?, ?, ?)""";
    try (PreparedStatement stmt = conn.prepareStatement(sql, Statement.RETURN_GENERATED_KEYS)) {
      stmt.setString(1, actor);
      stmt.setString(2, action);
      stmt.setString(3, entityType);
      stmt.setString(4, entityId);
      stmt.setObject(5, jsonb(beforeStateJson));
      stmt.setObject(6, jsonb(afterStateJson));
      stmt.executeUpdate();
      try (ResultSet keys = stmt.getGeneratedKeys()) {
        keys.next();
        return keys.getLong(1);
      }
    } catch (SQLException e) {
      throw new IllegalStateException("record audit_log failed", e);
    }
  }

  /** Convenience overload for callers with no in-flight {@link Connection} of their own. */
  public long record(
      String actor,
      String action,
      String entityType,
      String entityId,
      String beforeStateJson,
      String afterStateJson) {
    try (Connection conn = dataSource.getConnection()) {
      return record(conn, actor, action, entityType, entityId, beforeStateJson, afterStateJson);
    } catch (SQLException e) {
      throw new IllegalStateException("record audit_log failed", e);
    }
  }

  public List<AuditLogEntry> findByEntity(String entityType, String entityId) {
    String sql =
        "SELECT id, actor, action, entity_type, entity_id, before_state, after_state, created_at "
            + "FROM audit_log WHERE entity_type = ? AND entity_id = ? ORDER BY id";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, entityType);
      stmt.setString(2, entityId);
      try (ResultSet rs = stmt.executeQuery()) {
        List<AuditLogEntry> entries = new ArrayList<>();
        while (rs.next()) entries.add(toEntry(rs));
        return entries;
      }
    } catch (SQLException e) {
      throw new IllegalStateException("find audit_log by entity failed", e);
    }
  }

  /**
   * One page of an entity's audit rows, newest first; {@code beforeId} is the previous page's last
   * id (docs/04 §2 cursor convention), or null for the first page.
   */
  public List<AuditLogEntry> findPageByEntity(
      String entityType, String entityId, int limit, Long beforeId) {
    String sql =
        "SELECT id, actor, action, entity_type, entity_id, before_state, after_state, created_at "
            + "FROM audit_log WHERE entity_type = ? AND entity_id = ? AND (? IS NULL OR id < ?) "
            + "ORDER BY id DESC LIMIT ?";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, entityType);
      stmt.setString(2, entityId);
      stmt.setObject(3, beforeId, java.sql.Types.BIGINT);
      stmt.setObject(4, beforeId, java.sql.Types.BIGINT);
      stmt.setInt(5, limit);
      try (ResultSet rs = stmt.executeQuery()) {
        List<AuditLogEntry> entries = new ArrayList<>();
        while (rs.next()) entries.add(toEntry(rs));
        return entries;
      }
    } catch (SQLException e) {
      throw new IllegalStateException("find audit_log page failed", e);
    }
  }

  private static AuditLogEntry toEntry(ResultSet rs) throws SQLException {
    return new AuditLogEntry(
        rs.getLong("id"),
        rs.getString("actor"),
        rs.getString("action"),
        rs.getString("entity_type"),
        rs.getString("entity_id"),
        rs.getString("before_state"),
        rs.getString("after_state"),
        rs.getTimestamp("created_at").toInstant());
  }

  private static PGobject jsonb(String json) throws SQLException {
    PGobject obj = new PGobject();
    obj.setType("jsonb");
    obj.setValue(json);
    return obj;
  }
}
