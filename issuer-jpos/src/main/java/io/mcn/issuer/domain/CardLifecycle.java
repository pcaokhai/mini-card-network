package io.mcn.issuer.domain;

import java.time.LocalDate;
import java.time.YearMonth;
import java.util.Set;

/** Card status rules shared by the authorization path and the Admin API. */
public final class CardLifecycle {
  /** Statuses that say more than "expired" and so win over it on read. */
  private static final Set<String> OUTRANKS_EXPIRY = Set.of("LOST", "STOLEN", "PIN_BLOCKED");

  private CardLifecycle() {}

  /** A card is valid through the last day of its expiry month ({@code YYMM}). */
  public static boolean isExpired(String expiryYymm, LocalDate businessDate) {
    int year = 2000 + Integer.parseInt(expiryYymm.substring(0, 2));
    int month = Integer.parseInt(expiryYymm.substring(2, 4));
    return YearMonth.from(businessDate).isAfter(YearMonth.of(year, month));
  }

  /**
   * The status a reader sees: derived on read rather than written back by a batch job, so a GET
   * never mutates the card (CARDS-G4).
   */
  public static String effectiveStatus(
      String storedStatus, String expiryYymm, LocalDate businessDate) {
    if (OUTRANKS_EXPIRY.contains(storedStatus) || !isExpired(expiryYymm, businessDate)) {
      return storedStatus;
    }
    return "EXPIRED";
  }
}
