package io.mcn.issuer.adapter.persistence;

import java.sql.SQLException;
import java.time.LocalDate;
import javax.sql.DataSource;

/** Reads the ISO business date from {@code system_state} (docs/05 §2). */
public class BusinessDateRepository {
  private final DataSource dataSource;

  public BusinessDateRepository(DataSource dataSource) {
    this.dataSource = dataSource;
  }

  /**
   * The current business date. Falls back to the calendar date while no cutover has written {@code
   * system_state} - the same date the ISO path keys {@code velocity_counter} with until the cutover
   * epic moves it onto this row too.
   */
  public LocalDate current() {
    String sql = "SELECT current_business_date FROM system_state WHERE id = 1";
    try (var conn = dataSource.getConnection();
        var stmt = conn.prepareStatement(sql);
        var rs = stmt.executeQuery()) {
      return rs.next() ? rs.getObject(1, LocalDate.class) : LocalDate.now();
    } catch (SQLException e) {
      throw new IllegalStateException("read system_state business date failed", e);
    }
  }
}
