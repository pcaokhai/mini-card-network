package io.mcn.issuer.adapter.http;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.javalin.config.RoutesConfig;
import io.javalin.http.Context;
import io.mcn.issuer.adapter.http.ApiProblems.FieldError;
import io.mcn.issuer.adapter.persistence.AccountRepository;
import io.mcn.issuer.adapter.persistence.AccountSummary;
import io.mcn.issuer.adapter.persistence.AuditLogEntry;
import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.BusinessDateRepository;
import io.mcn.issuer.adapter.persistence.Card;
import io.mcn.issuer.adapter.persistence.CardLimit;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.IdempotencyRecord;
import io.mcn.issuer.adapter.persistence.IdempotencyRepository;
import io.mcn.issuer.adapter.persistence.JournalEntryRow;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.domain.BlockReason;
import io.mcn.issuer.domain.CardLifecycle;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.sql.Connection;
import java.sql.SQLException;
import java.time.LocalDate;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.regex.Pattern;
import javax.sql.DataSource;

/**
 * `/v1/cards*` Admin API (MCN-308): list/detail, block/unblock, limits (If-Match), ledger, audit.
 * Registers into the caller's {@link RoutesConfig} rather than owning a Javalin instance - Javalin
 * 7 only exposes route registration inside {@code Javalin.create(config -> ...)}, so this shares
 * the one app/port the issuer Admin API already runs on (docs/04 §1: issuer:8081), same as {@link
 * HealthServer}.
 *
 * <p>Card ETags version the limits (that is what If-Match protects); balances and status are never
 * served from a cache because every response here is {@code Cache-Control: no-store}.
 */
public final class CardAdminController {
  private static final Pattern CARD_REF = Pattern.compile("^crd_[A-Za-z0-9]{10,32}$");
  private static final Pattern UUID_KEY =
      Pattern.compile(
          "^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$");
  private static final int DEFAULT_PAGE_SIZE = 50;
  private static final int MAX_PAGE_SIZE = 200;

  private final DataSource dataSource;
  private final CardRepository cards;
  private final AccountRepository accounts;
  private final CardLimitRepository cardLimits;
  private final AuditLogRepository auditLog;
  private final IdempotencyRepository idempotency;
  private final LedgerRepository ledger;
  private final BusinessDateRepository businessDates;
  private final ObjectMapper mapper = new ObjectMapper();

  private record Paging(int limit, Long cursor) {}

  /**
   * The reads a {@code CardDetail} needs besides the card and its limits, taken before a write
   * opens its transaction: each uses its own pooled connection, and a request holding the card's
   * row lock must never wait for a second one (N writers queued on the lock could hold the pool).
   */
  private record CardReads(LocalDate businessDate, AccountSummary account, long usedToday) {}

  private CardReads readsFor(Card card) {
    LocalDate businessDate = businessDates.current();
    return new CardReads(
        businessDate,
        accounts.findById(card.accountId()).orElseThrow(),
        cardLimits.amountToday(card.id(), businessDate));
  }

  public CardAdminController(
      DataSource dataSource,
      CardRepository cards,
      AccountRepository accounts,
      CardLimitRepository cardLimits,
      AuditLogRepository auditLog,
      IdempotencyRepository idempotency,
      LedgerRepository ledger,
      BusinessDateRepository businessDates) {
    this.dataSource = dataSource;
    this.cards = cards;
    this.accounts = accounts;
    this.cardLimits = cardLimits;
    this.auditLog = auditLog;
    this.idempotency = idempotency;
    this.ledger = ledger;
    this.businessDates = businessDates;
  }

  public void registerRoutes(RoutesConfig routes) {
    routes
        .before("/v1/cards", CardAdminController::noStore)
        .before("/v1/cards/*", CardAdminController::noStore)
        .before("/v1/cards/{cardRef}", CardAdminController::rejectMalformedCardRef)
        .before("/v1/cards/{cardRef}/{resource}", CardAdminController::rejectMalformedCardRef)
        .get("/v1/cards", this::listCards)
        .get("/v1/cards/{cardRef}", this::getCard)
        .post("/v1/cards/{cardRef}/blocks", ctx -> changeBlockStatus(ctx, true))
        .delete("/v1/cards/{cardRef}/blocks", ctx -> changeBlockStatus(ctx, false))
        .put("/v1/cards/{cardRef}/limits", this::updateLimits)
        .get("/v1/cards/{cardRef}/ledger", this::getLedger)
        .get("/v1/cards/{cardRef}/audit", this::getAudit);
  }

  private static void noStore(Context ctx) {
    ctx.header("Cache-Control", "no-store");
  }

  private static void rejectMalformedCardRef(Context ctx) {
    if (CARD_REF.matcher(ctx.pathParam("cardRef")).matches()) return;
    ApiProblems.validation(
        ctx, List.of(new FieldError("cardRef", "must match " + CARD_REF.pattern())));
    ctx.skipRemainingHandlers();
  }

  private void listCards(Context ctx) {
    LocalDate businessDate = businessDates.current();
    ctx.json(cards.findAll().stream().map(c -> cardSummary(c, businessDate)).toList());
  }

  private void getCard(Context ctx) {
    Optional<Card> card = cards.findByCardRef(ctx.pathParam("cardRef"));
    if (card.isEmpty()) {
      notFound(ctx);
      return;
    }
    List<CardLimit> limits = cardLimits.findAllForCard(card.get().id());
    Map<String, Object> body = cardDetail(card.get(), limits, readsFor(card.get()));
    ctx.header("ETag", etagFor(limits, body));
    ctx.json(body);
  }

  /** Shared POST/DELETE .../blocks handler; {@code block} picks which state transition runs. */
  private void changeBlockStatus(Context ctx, boolean block) {
    String idempotencyKey = ctx.header("Idempotency-Key");
    if (idempotencyKey == null || !UUID_KEY.matcher(idempotencyKey).matches()) {
      insufficientIdempotencyKey(ctx);
      return;
    }
    String route = ctx.method() + " " + ctx.path();
    String requestHash = sha256(ctx.body());
    if (replayIfPresent(ctx, idempotency.find(idempotencyKey, route), requestHash)) return;

    Optional<BlockReason> reason = block ? blockReason(ctx.body()) : Optional.empty();
    if (block && reason.isEmpty()) {
      ApiProblems.validation(
          ctx,
          List.of(
              new FieldError(
                  "reason", "must be one of CUSTOMER_REQUEST, LOST, STOLEN, FRAUD_SUSPECTED")));
      return;
    }

    Optional<Card> card = cards.findByCardRef(ctx.pathParam("cardRef"));
    if (card.isEmpty()) {
      notFound(ctx);
      return;
    }
    CardReads reads = readsFor(card.get());
    try (Connection conn = dataSource.getConnection()) {
      conn.setAutoCommit(false);
      transitionUnderLock(ctx, conn, reason, reads, idempotencyKey, route, requestHash);
    } catch (SQLException e) {
      throw new IllegalStateException("block/unblock card failed", e);
    }
  }

  /**
   * Checks and changes the status under the card's row lock, so two operators can't both pass the
   * "is it ACTIVE?" check. An empty {@code reason} means unblock.
   */
  private void transitionUnderLock(
      Context ctx,
      Connection conn,
      Optional<BlockReason> reason,
      CardReads reads,
      String idempotencyKey,
      String route,
      String requestHash)
      throws SQLException {
    Optional<Card> locked = cards.lockByCardRef(conn, ctx.pathParam("cardRef"));
    if (locked.isEmpty()) {
      conn.rollback();
      notFound(ctx);
      return;
    }
    // Same-key requests in flight together queue on this lock; the first commits its record before
    // releasing it, so the rest replay it here instead of hitting its primary key.
    if (replayIfPresent(ctx, idempotency.find(conn, idempotencyKey, route), requestHash)) {
      conn.rollback();
      return;
    }
    Card card = locked.get();
    String current =
        CardLifecycle.effectiveStatus(card.status(), card.expiryYymm(), reads.businessDate());
    boolean block = reason.isPresent();
    if (!current.equals(block ? "ACTIVE" : "BLOCKED")) {
      conn.rollback();
      conflict(ctx, card.cardRef(), current, block);
      return;
    }
    String newStatus = reason.map(BlockReason::cardStatus).orElse("ACTIVE");
    Map<String, Object> after = new LinkedHashMap<>();
    after.put("status", newStatus);
    reason.ifPresent(r -> after.put("reason", r.name()));

    cards.updateStatus(conn, card.id(), newStatus);
    auditLog.record(
        conn,
        actor(ctx),
        block ? "CARD_BLOCKED" : "CARD_UNBLOCKED",
        "card",
        card.cardRef(),
        jsonOf(Map.of("status", current)),
        jsonOf(after));
    List<CardLimit> limits = cardLimits.findAllForCard(conn, card.id());
    Map<String, Object> body = cardDetail(withStatus(card, newStatus), limits, reads);
    idempotency.store(conn, idempotencyKey, route, requestHash, 200, jsonOf(body));
    conn.commit();
    ctx.status(200).json(body);
  }

  private void updateLimits(Context ctx) {
    String idempotencyKey = ctx.header("Idempotency-Key");
    if (idempotencyKey == null || !UUID_KEY.matcher(idempotencyKey).matches()) {
      insufficientIdempotencyKey(ctx);
      return;
    }
    // Replay before If-Match: a retried successful save carries the ETag it just made stale.
    String route = ctx.method() + " " + ctx.path();
    String requestHash = sha256(ctx.body());
    if (replayIfPresent(ctx, idempotency.find(idempotencyKey, route), requestHash)) return;

    Optional<Card> card = cards.findByCardRef(ctx.pathParam("cardRef"));
    if (card.isEmpty()) {
      notFound(ctx);
      return;
    }
    CardReads reads = readsFor(card.get());
    switch (CardLimitsRequest.parse(ctx.body(), reads.account().currency())) {
      case CardLimitsRequest.Invalid invalid -> ApiProblems.validation(ctx, invalid.errors());
      case CardLimitsRequest.Valid valid -> {
        try (Connection conn = dataSource.getConnection()) {
          conn.setAutoCommit(false);
          saveLimitsUnderLock(
              ctx, conn, valid.request(), reads, idempotencyKey, route, requestHash);
        } catch (SQLException e) {
          throw new IllegalStateException("update card limits failed", e);
        }
      }
    }
  }

  /**
   * Compares If-Match with the limits read under the card's row lock and upserts in the same
   * transaction: of two concurrent PUTs carrying one ETag, the second sees the first's limits and
   * gets 412 (CARDS-G6).
   */
  private void saveLimitsUnderLock(
      Context ctx,
      Connection conn,
      CardLimitsRequest request,
      CardReads reads,
      String idempotencyKey,
      String route,
      String requestHash)
      throws SQLException {
    Card card = cards.lockByCardRef(conn, ctx.pathParam("cardRef")).orElseThrow();
    // As in transitionUnderLock: a same-key request that was in flight with this one replays here.
    if (replayIfPresent(ctx, idempotency.find(conn, idempotencyKey, route), requestHash)) {
      conn.rollback();
      return;
    }
    List<CardLimit> currentLimits = cardLimits.findAllForCard(conn, card.id());
    if (!ifMatchAccepts(ctx.header("If-Match"), currentLimits)) {
      conn.rollback();
      preconditionFailed(ctx);
      return;
    }
    cardLimits.upsertAllLimits(
        conn,
        card.id(),
        request.perTransactionAmount(),
        request.dailyAmount(),
        request.dailyCount());
    // ponytail: built in memory rather than re-read - the upsert above is what's now stored.
    List<CardLimit> newLimits =
        List.of(
            new CardLimit("ALL", "PER_TXN", request.perTransactionAmount(), null),
            new CardLimit("ALL", "DAILY", request.dailyAmount(), request.dailyCount()));
    String currency = reads.account().currency();
    auditLog.record(
        conn,
        actor(ctx),
        "CARD_LIMITS_UPDATED",
        "card",
        card.cardRef(),
        jsonOf(limitsAsMap(currentLimits, currency)),
        jsonOf(limitsAsMap(newLimits, currency)));
    Map<String, Object> body = cardDetail(card, newLimits, reads);
    idempotency.store(conn, idempotencyKey, route, requestHash, 200, jsonOf(body));
    conn.commit();
    ctx.header("ETag", etagFor(newLimits, body));
    ctx.status(200).json(body);
  }

  private void getLedger(Context ctx) {
    Optional<Paging> paging = paging(ctx);
    if (paging.isEmpty()) return;
    Optional<Card> card = cards.findByCardRef(ctx.pathParam("cardRef"));
    if (card.isEmpty()) {
      notFound(ctx);
      return;
    }
    int limit = paging.get().limit();
    List<JournalEntryRow> rows =
        ledger.findByAccount(dataSource, card.get().accountId(), limit, paging.get().cursor());
    String nextCursor =
        rows.size() == limit ? String.valueOf(rows.get(rows.size() - 1).journalId()) : null;
    ctx.json(page(rows.stream().map(this::journalEntry).toList(), nextCursor));
  }

  private void getAudit(Context ctx) {
    Optional<Paging> paging = paging(ctx);
    if (paging.isEmpty()) return;
    String cardRef = ctx.pathParam("cardRef");
    if (cards.findByCardRef(cardRef).isEmpty()) {
      notFound(ctx);
      return;
    }
    int limit = paging.get().limit();
    List<AuditLogEntry> rows =
        auditLog.findPageByEntity("card", cardRef, limit, paging.get().cursor());
    String nextCursor =
        rows.size() == limit ? String.valueOf(rows.get(rows.size() - 1).id()) : null;
    ctx.json(page(rows.stream().map(this::auditEntry).toList(), nextCursor));
  }

  /** {@code limit} 1-200 (default 50) and a positive integer {@code cursor} (CARDS-G8). */
  private static Optional<Paging> paging(Context ctx) {
    String rawLimit = ctx.queryParam("limit");
    String rawCursor = ctx.queryParam("cursor");
    Optional<Long> limit =
        rawLimit == null ? Optional.of((long) DEFAULT_PAGE_SIZE) : positiveLong(rawLimit);
    Optional<Long> cursor = rawCursor == null ? Optional.empty() : positiveLong(rawCursor);
    List<FieldError> errors = new ArrayList<>();
    if (limit.isEmpty() || limit.get() > MAX_PAGE_SIZE) {
      errors.add(new FieldError("limit", "must be an integer from 1 to " + MAX_PAGE_SIZE));
    }
    if (rawCursor != null && cursor.isEmpty()) {
      errors.add(new FieldError("cursor", "must be a nextCursor from a previous page"));
    }
    if (!errors.isEmpty()) {
      ApiProblems.validation(ctx, errors);
      return Optional.empty();
    }
    return Optional.of(new Paging(limit.get().intValue(), cursor.orElse(null)));
  }

  private static Optional<Long> positiveLong(String raw) {
    try {
      long value = Long.parseLong(raw);
      return value > 0 ? Optional.of(value) : Optional.empty();
    } catch (NumberFormatException e) {
      return Optional.empty();
    }
  }

  private Optional<BlockReason> blockReason(String body) {
    try {
      return BlockReason.parse(mapper.readTree(body).path("reason").asText(null));
    } catch (Exception e) {
      return Optional.empty();
    }
  }

  // ---- replay / idempotency ----------------------------------------------------------------

  /** Answers from the stored record (or 422 on a body mismatch); true when it answered. */
  private boolean replayIfPresent(
      Context ctx, Optional<IdempotencyRecord> stored, String requestHash) {
    if (stored.isEmpty()) return false;
    IdempotencyRecord record = stored.get();
    if (!record.requestHash().equals(requestHash)) {
      idempotencyKeyMismatch(ctx);
      return true;
    }
    ctx.status(record.status()).contentType("application/json").result(record.body());
    return true;
  }

  // ---- response shaping ----------------------------------------------------------------------

  private Map<String, Object> cardSummary(Card card, LocalDate businessDate) {
    Map<String, Object> m = new LinkedHashMap<>();
    m.put("cardRef", card.cardRef());
    m.put("maskedPan", maskedPan(card));
    m.put("holderName", card.holderName());
    m.put("status", CardLifecycle.effectiveStatus(card.status(), card.expiryYymm(), businessDate));
    m.put("expiry", expiryDisplay(card.expiryYymm()));
    return m;
  }

  private Map<String, Object> cardDetail(Card card, List<CardLimit> limits, CardReads reads) {
    Map<String, Object> m = cardSummary(card, reads.businessDate());
    AccountSummary account = reads.account();
    m.put("ledgerBalance", money(account.ledgerBalance(), account.currency()));
    m.put("availableBalance", money(account.availableBalance(), account.currency()));
    // ponytail: no PREAUTH flow ships yet (MCN-603), so auth_hold is always empty for now.
    m.put("holds", List.of());
    m.put("limits", limitsAsMap(limits, account.currency()));
    m.put("usedToday", money(reads.usedToday(), account.currency()));
    return m;
  }

  private static Card withStatus(Card card, String status) {
    return new Card(
        card.id(),
        card.accountId(),
        card.bin(),
        card.panLast4(),
        card.expiryYymm(),
        status,
        card.cardRef(),
        card.holderName());
  }

  private Map<String, Object> limitsAsMap(List<CardLimit> limits, String currency) {
    Long perTxn = null;
    Long daily = null;
    Integer dailyCount = null;
    for (CardLimit l : limits) {
      if ("PER_TXN".equals(l.period())) perTxn = l.maxAmount();
      if ("DAILY".equals(l.period())) {
        daily = l.maxAmount();
        dailyCount = l.maxCount();
      }
    }
    Map<String, Object> m = new LinkedHashMap<>();
    m.put("dailyAmount", money(daily == null ? 0 : daily, currency));
    m.put("perTransactionAmount", money(perTxn == null ? 0 : perTxn, currency));
    m.put("dailyCount", dailyCount);
    return m;
  }

  private Map<String, Object> journalEntry(JournalEntryRow row) {
    Map<String, Object> m = new LinkedHashMap<>();
    m.put("journalId", String.valueOf(row.journalId()));
    m.put("occurredAt", row.occurredAt().toString());
    m.put("description", row.entryType() + " journal " + row.journalId());
    m.put("entryType", row.entryType());
    m.put("rrn", row.rrn());
    List<Map<String, Object>> postings = new ArrayList<>();
    for (var p : row.postings()) {
      Map<String, Object> posting = new LinkedHashMap<>();
      posting.put("account", p.account());
      posting.put("direction", "D".equals(p.direction()) ? "DEBIT" : "CREDIT");
      posting.put("amount", money(p.amount(), p.currency()));
      postings.add(posting);
    }
    m.put("postings", postings);
    return m;
  }

  private Map<String, Object> auditEntry(AuditLogEntry row) {
    Map<String, Object> m = new LinkedHashMap<>();
    m.put("auditId", String.valueOf(row.id()));
    m.put("occurredAt", row.createdAt().toString());
    m.put("actor", row.actor());
    m.put("action", row.action());
    m.put("before", jsonTree(row.beforeState()));
    m.put("after", jsonTree(row.afterState()));
    return m;
  }

  private static Map<String, Object> page(List<Map<String, Object>> items, String nextCursor) {
    Map<String, Object> body = new LinkedHashMap<>();
    body.put("items", items);
    body.put("nextCursor", nextCursor);
    return body;
  }

  private static Map<String, Object> money(long amount, String currency) {
    Map<String, Object> m = new LinkedHashMap<>();
    m.put("amount", amount);
    m.put("currency", currency);
    return m;
  }

  // ponytail: fixture PANs are all 16 digits (BIN 970436); a real issuer would mask by the
  // stored PAN length instead of assuming 16.
  private static String maskedPan(Card card) {
    return card.bin()
        + "*".repeat(16 - card.bin().length() - card.panLast4().length())
        + card.panLast4();
  }

  private static String expiryDisplay(String yymm) {
    return yymm.substring(2, 4) + "/" + yymm.substring(0, 2);
  }

  // ---- ETag: "<limits version>-<representation hash>" ------------------------------------------

  /**
   * The first part versions the limits and is all If-Match compares; the second covers the whole
   * body, so a direct caller's If-None-Match never gets a 304 after a balance or status change.
   */
  private String etagFor(List<CardLimit> limits, Map<String, Object> body) {
    return '"' + limitsVersion(limits) + '-' + sha256(jsonOf(body)).substring(0, 16) + '"';
  }

  private static boolean ifMatchAccepts(String ifMatch, List<CardLimit> currentLimits) {
    if (ifMatch == null) return false;
    String opaque = ifMatch.trim().replace("\"", "");
    int dash = opaque.indexOf('-');
    String version = dash < 0 ? opaque : opaque.substring(0, dash);
    return version.equals(limitsVersion(currentLimits));
  }

  private static String limitsVersion(List<CardLimit> limits) {
    StringBuilder sb = new StringBuilder();
    limits.stream()
        .sorted((a, b) -> a.period().compareTo(b.period()))
        .forEach(
            l ->
                sb.append(l.period())
                    .append(':')
                    .append(l.maxAmount())
                    .append(':')
                    .append(l.maxCount())
                    .append(';'));
    return sha256(sb.toString());
  }

  private static String actor(Context ctx) {
    String actor = ctx.header("X-Actor");
    return actor == null || actor.isBlank() ? "unknown" : actor;
  }

  private String jsonOf(Object value) {
    try {
      return mapper.writeValueAsString(value);
    } catch (Exception e) {
      throw new IllegalStateException("serialize json failed", e);
    }
  }

  private Object jsonTree(String json) {
    try {
      return json == null ? null : mapper.readValue(json, Map.class);
    } catch (Exception e) {
      throw new IllegalStateException("parse audit json failed", e);
    }
  }

  private static String sha256(String input) {
    try {
      byte[] digest =
          MessageDigest.getInstance("SHA-256").digest(input.getBytes(StandardCharsets.UTF_8));
      StringBuilder hex = new StringBuilder();
      for (byte b : digest) hex.append(String.format("%02x", b));
      return hex.toString();
    } catch (Exception e) {
      throw new IllegalStateException("sha256 failed", e);
    }
  }

  // ---- problems (shaped by ApiProblems) --------------------------------------------------------

  private static void notFound(Context ctx) {
    ApiProblems.send(
        ctx, 404, "not-found", "card not found", "card not found: " + ctx.pathParam("cardRef"));
  }

  private static void conflict(Context ctx, String cardRef, String status, boolean block) {
    ApiProblems.send(
        ctx,
        409,
        "conflict",
        "State transition not allowed",
        "card " + cardRef + " is " + status + ", cannot " + (block ? "block" : "unblock") + " it");
  }

  private static void insufficientIdempotencyKey(Context ctx) {
    ApiProblems.send(
        ctx,
        400,
        "insufficient-idempotency-key",
        "Idempotency-Key missing or not a UUID",
        ctx.method() + " " + ctx.path() + " requires an Idempotency-Key that is a UUID.");
  }

  private static void idempotencyKeyMismatch(Context ctx) {
    ApiProblems.send(
        ctx,
        422,
        "idempotency-key-mismatch",
        "Idempotency-Key reused with a different request",
        "The same Idempotency-Key was used with a different request body.");
  }

  private static void preconditionFailed(Context ctx) {
    ApiProblems.send(
        ctx,
        412,
        "precondition-failed",
        "ETag mismatch",
        "If-Match does not match the card's current limits version.");
  }
}
