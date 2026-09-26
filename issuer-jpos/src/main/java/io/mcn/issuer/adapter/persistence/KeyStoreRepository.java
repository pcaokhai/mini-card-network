package io.mcn.issuer.adapter.persistence;

import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Optional;
import javax.sql.DataSource;

/** Reads/writes {@code key_store}; keys are always cryptograms under LMK, never clear. */
public class KeyStoreRepository {

  /** Dual-key acceptance window (docs/03 §9's "Reversal grace for new key (DE 70 = 161)"). */
  public static final Duration DUAL_KEY_WINDOW = Duration.ofMinutes(5);

  private final DataSource dataSource;

  public KeyStoreRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  public long insert(KeyStoreRow row) {
    String sql =
        """
        INSERT INTO key_store (key_type, counterparty, key_under_lmk, kcv, status)
        VALUES (?, ?, ?, ?, 'PENDING') RETURNING id""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, row.keyType());
      stmt.setString(2, row.counterparty());
      stmt.setString(3, row.keyUnderLmkHex());
      stmt.setString(4, row.kcv());
      try (ResultSet rs = stmt.executeQuery()) {
        rs.next();
        return rs.getLong("id");
      }
    } catch (SQLException e) {
      throw new IllegalStateException("insert key_store failed", e);
    }
  }

  /**
   * Retires any other ACTIVE row sharing {@code (key_type, counterparty)}, then activates {@code
   * id}.
   */
  public void activate(long id) {
    String retirePrevious =
        """
        UPDATE key_store SET status = 'RETIRED', retired_at = now()
        WHERE key_type = (SELECT key_type FROM key_store WHERE id = ?)
          AND COALESCE(counterparty, '') = (SELECT COALESCE(counterparty, '') FROM key_store WHERE id = ?)
          AND status = 'ACTIVE'""";
    String activate = "UPDATE key_store SET status = 'ACTIVE', activated_at = now() WHERE id = ?";
    try (var conn = dataSource.getConnection()) {
      conn.setAutoCommit(false);
      try (PreparedStatement retireStmt = conn.prepareStatement(retirePrevious)) {
        retireStmt.setLong(1, id);
        retireStmt.setLong(2, id);
        retireStmt.executeUpdate();
      }
      try (PreparedStatement activateStmt = conn.prepareStatement(activate)) {
        activateStmt.setLong(1, id);
        activateStmt.executeUpdate();
      }
      conn.commit();
    } catch (SQLException e) {
      throw new IllegalStateException("activate key_store failed", e);
    }
  }

  /**
   * Dual-key acceptance window (MCN-504-AC2): the most recently {@code RETIRED} row for {@code
   * (keyType, counterparty)}, if it retired within {@code within} of now - a time-boxed read, not a
   * second {@code ACTIVE} row, per {@code MCN-504-GW.md}'s Ruling 2. A row that was never activated
   * is never returned, whatever its status (SEC-G15, as the gateway's S1 fix).
   */
  public Optional<KeyStoreRow> findRecentlyRetired(
      String keyType, String counterparty, Duration within) {
    String sql =
        """
        SELECT id, key_type, counterparty, key_under_lmk, kcv, status, activated_at, retired_at,
               created_at
        FROM key_store
        WHERE key_type = ? AND COALESCE(counterparty, '') = ? AND status = 'RETIRED'
          AND activated_at IS NOT NULL
          AND retired_at > now() - (? || ' seconds')::interval
        ORDER BY retired_at DESC LIMIT 1""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, keyType);
      stmt.setString(2, counterparty == null ? "" : counterparty);
      stmt.setLong(3, within.toSeconds());
      try (ResultSet rs = stmt.executeQuery()) {
        return rs.next() ? Optional.of(mapRow(rs)) : Optional.empty();
      }
    } catch (SQLException e) {
      throw new IllegalStateException("find recently retired key_store row failed", e);
    }
  }

  /** The ACTIVE row for {@code (keyType, counterparty)} (at most one: {@code V8}). */
  public Optional<KeyStoreRow> findActive(String keyType, String counterparty) {
    String sql =
        """
        SELECT id, key_type, counterparty, key_under_lmk, kcv, status, activated_at, retired_at,
               created_at
        FROM key_store
        WHERE key_type = ? AND COALESCE(counterparty, '') = ? AND status = 'ACTIVE'""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, keyType);
      stmt.setString(2, counterparty == null ? "" : counterparty);
      try (ResultSet rs = stmt.executeQuery()) {
        return rs.next() ? Optional.of(mapRow(rs)) : Optional.empty();
      }
    } catch (SQLException e) {
      throw new IllegalStateException("find active key_store row failed", e);
    }
  }

  /**
   * Inserts {@code row} as ACTIVE unless an ACTIVE key of that type and counterparty exists
   * (startup seeding, like the gateway's {@code EnsureActive}); the partial unique index makes a
   * concurrent seed a no-op. Returns whether this call inserted it.
   */
  public boolean ensureActive(KeyStoreRow row) {
    String sql =
        """
        INSERT INTO key_store (key_type, counterparty, key_under_lmk, kcv, status, activated_at)
        VALUES (?, ?, ?, ?, 'ACTIVE', now())
        ON CONFLICT (key_type, COALESCE(counterparty, '')) WHERE status = 'ACTIVE' DO NOTHING""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, row.keyType());
      stmt.setString(2, row.counterparty());
      stmt.setString(3, row.keyUnderLmkHex());
      stmt.setString(4, row.kcv());
      return stmt.executeUpdate() == 1;
    } catch (SQLException e) {
      throw new IllegalStateException("ensure active key_store row failed", e);
    }
  }

  /** Every RETIRED row for {@code (keyType, counterparty)}, newest first (replay check). */
  public List<KeyStoreRow> findRetired(String keyType, String counterparty) {
    String sql =
        """
        SELECT id, key_type, counterparty, key_under_lmk, kcv, status, activated_at, retired_at,
               created_at
        FROM key_store
        WHERE key_type = ? AND COALESCE(counterparty, '') = ? AND status = 'RETIRED'
        ORDER BY id DESC""";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql)) {
      stmt.setString(1, keyType);
      stmt.setString(2, counterparty == null ? "" : counterparty);
      try (ResultSet rs = stmt.executeQuery()) {
        List<KeyStoreRow> rows = new ArrayList<>();
        while (rs.next()) {
          rows.add(mapRow(rs));
        }
        return rows;
      }
    } catch (SQLException e) {
      throw new IllegalStateException("find retired key_store rows failed", e);
    }
  }

  public List<KeyStoreRow> findAll() {
    String sql =
        "SELECT id, key_type, counterparty, key_under_lmk, kcv, status, activated_at, retired_at, "
            + "created_at FROM key_store ORDER BY id";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql);
        var rs = stmt.executeQuery()) {
      List<KeyStoreRow> rows = new ArrayList<>();
      while (rs.next()) {
        rows.add(mapRow(rs));
      }
      return rows;
    } catch (SQLException e) {
      throw new IllegalStateException("find key_store failed", e);
    }
  }

  private static KeyStoreRow mapRow(ResultSet rs) throws SQLException {
    return new KeyStoreRow(
        rs.getLong("id"),
        rs.getString("key_type"),
        rs.getString("counterparty"),
        rs.getString("key_under_lmk"),
        rs.getString("kcv"),
        rs.getString("status"),
        toInstant(rs.getTimestamp("activated_at")),
        toInstant(rs.getTimestamp("retired_at")),
        toInstant(rs.getTimestamp("created_at")));
  }

  private static Instant toInstant(Timestamp ts) {
    return ts == null ? null : ts.toInstant();
  }
}
