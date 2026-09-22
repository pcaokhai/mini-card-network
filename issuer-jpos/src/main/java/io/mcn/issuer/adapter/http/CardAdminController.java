package io.mcn.issuer.adapter.http;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.javalin.config.RoutesConfig;
import io.javalin.http.Context;
import io.mcn.issuer.adapter.persistence.AccountRepository;
import io.mcn.issuer.adapter.persistence.AccountSummary;
import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.Card;
import io.mcn.issuer.adapter.persistence.CardLimit;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.IdempotencyRecord;
import io.mcn.issuer.adapter.persistence.IdempotencyRepository;
import io.mcn.issuer.adapter.persistence.JournalEntryRow;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.sql.Connection;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;
import javax.sql.DataSource;

/**
 * `/v1/cards*` Admin API (MCN-308): list/detail, block/unblock, limits (If-Match), ledger.
 * Registers into the caller's {@link RoutesConfig} rather than owning a Javalin instance - Javalin
 * 7 only exposes route registration inside {@code Javalin.create(config -> ...)}, so this shares
 * the one app/port the issuer Admin API already runs on (docs/04 §1: issuer:8081), same as {@link
 * HealthServer}.
 */
public final class CardAdminController {
  private final DataSource dataSource;
  private final CardRepository cards;
  private final AccountRepository accounts;
  private final CardLimitRepository cardLimits;
  private final AuditLogRepository auditLog;
  private final IdempotencyRepository idempotency;
  private final LedgerRepository ledger;
  private final ObjectMapper mapper = new ObjectMapper();

  public CardAdminController(
      DataSource dataSource,
      CardRepository cards,
      AccountRepository accounts,
      CardLimitRepository cardLimits,
      AuditLogRepository auditLog,
      IdempotencyRepository idempotency,
      LedgerRepository ledger) {
    this.dataSource = dataSource;
    this.cards = cards;
    this.accounts = accounts;
    this.cardLimits = cardLimits;
    this.auditLog = auditLog;
    this.idempotency = idempotency;
    this.ledger = ledger;
  }

  public void registerRoutes(RoutesConfig routes) {
    routes
        .get("/v1/cards", this::listCards)
        .get("/v1/cards/{cardRef}", this::getCard)
        .post("/v1/cards/{cardRef}/blocks", ctx -> changeBlockStatus(ctx, true))
        .delete("/v1/cards/{cardRef}/blocks", ctx -> changeBlockStatus(ctx, false))
        .put("/v1/cards/{cardRef}/limits", this::updateLimits)
        .get("/v1/cards/{cardRef}/ledger", this::getLedger);
  }

  private void listCards(Context ctx) {
    List<Map<String, Object>> summaries = cards.findAll().stream().map(this::cardSummary).toList();
    ctx.json(summaries);
  }

  private void getCard(Context ctx) {
    Optional<Card> card = cards.findByCardRef(ctx.pathParam("cardRef"));
    if (card.isEmpty()) {
      notFound(ctx, "card");
      return;
    }
    List<CardLimit> limits = cardLimits.findAllForCard(card.get().id());
    ctx.header("ETag", etagFor(limits));
    ctx.json(cardDetail(card.get(), limits));
  }

  /** Shared POST/DELETE .../blocks handler; {@code block} picks which state transition runs. */
  private void changeBlockStatus(Context ctx, boolean block) {
    String idempotencyKey = ctx.header("Idempotency-Key");
    if (idempotencyKey == null || idempotencyKey.isBlank()) {
      insufficientIdempotencyKey(ctx);
      return;
    }
    String route = ctx.method() + " " + ctx.path();
    String requestHash = sha256(ctx.body());

    Optional<IdempotencyRecord> replay = replayIfPresent(ctx, route, idempotencyKey, requestHash);
    if (replay.isPresent()) return;

    Optional<Card> maybeCard = cards.findByCardRef(ctx.pathParam("cardRef"));
    if (maybeCard.isEmpty()) {
      notFound(ctx, "card");
      return;
    }
    Card card = maybeCard.get();
    String newStatus = block ? "BLOCKED" : "ACTIVE";
    String errorIfNot = block ? "ACTIVE" : "BLOCKED";
    if (!card.status().equals(errorIfNot)) {
      conflict(
          ctx,
          "card "
              + card.cardRef()
              + " is "
              + card.status()
              + ", cannot "
              + (block ? "block" : "unblock")
              + " it");
      return;
    }
    String action = block ? "CARD_BLOCKED" : "CARD_UNBLOCKED";
    String actor = actor(ctx);

    Map<String, Object> responseBody;
    try (Connection conn = dataSource.getConnection()) {
      conn.setAutoCommit(false);
      cards.updateStatus(conn, card.id(), newStatus);
      auditLog.record(
          conn,
          actor,
          action,
          "card",
          card.cardRef(),
          jsonOf(Map.of("status", card.status())),
          jsonOf(Map.of("status", newStatus)));
      // ponytail: build the updated Card in memory rather than re-reading it here - a re-read
      // would use a fresh connection and, under READ COMMITTED, not see this transaction's own
      // uncommitted UPDATE yet.
      Card updated =
          new Card(
              card.id(),
              card.accountId(),
              card.bin(),
              card.panLast4(),
              card.expiryYymm(),
              newStatus,
              card.cardRef(),
              card.holderName());
      responseBody = cardDetail(updated, cardLimits.findAllForCard(updated.id()));
      idempotency.store(conn, idempotencyKey, route, requestHash, 200, jsonOf(responseBody));
      conn.commit();
    } catch (java.sql.SQLException e) {
      throw new IllegalStateException("block/unblock card failed", e);
    }
    ctx.status(200).json(responseBody);
  }

  private void updateLimits(Context ctx) {
    String idempotencyKey = ctx.header("Idempotency-Key");
    if (idempotencyKey == null || idempotencyKey.isBlank()) {
      insufficientIdempotencyKey(ctx);
      return;
    }
    Optional<Card> maybeCard = cards.findByCardRef(ctx.pathParam("cardRef"));
    if (maybeCard.isEmpty()) {
      notFound(ctx, "card");
      return;
    }
    Card card = maybeCard.get();
    List<CardLimit> currentLimits = cardLimits.findAllForCard(card.id());
    String ifMatch = ctx.header("If-Match");
    if (ifMatch == null || !ifMatch.equals(etagFor(currentLimits))) {
      preconditionFailed(ctx);
      return;
    }

    JsonNode body;
    try {
      body = mapper.readTree(ctx.body());
    } catch (Exception e) {
      validationError(ctx, "body must be valid JSON");
      return;
    }
    long perTxnAmount = body.path("perTransactionAmount").path("amount").asLong(-1);
    long dailyAmount = body.path("dailyAmount").path("amount").asLong(-1);
    Integer dailyCount =
        body.path("dailyCount").isMissingNode() || body.path("dailyCount").isNull()
            ? null
            : body.path("dailyCount").asInt();
    if (perTxnAmount <= 0 || dailyAmount <= 0) {
      validationError(ctx, "dailyAmount.amount and perTransactionAmount.amount must be > 0");
      return;
    }

    String route = ctx.method() + " " + ctx.path();
    String requestHash = sha256(ctx.body());
    Optional<IdempotencyRecord> replay = replayIfPresent(ctx, route, idempotencyKey, requestHash);
    if (replay.isPresent()) return;

    // ponytail: computed in memory instead of re-read post-upsert - a re-read would use a fresh
    // connection and, under READ COMMITTED, not see this transaction's own uncommitted upsert yet.
    List<CardLimit> newLimits =
        List.of(
            new CardLimit("ALL", "PER_TXN", perTxnAmount, null),
            new CardLimit("ALL", "DAILY", dailyAmount, dailyCount));

    Map<String, Object> responseBody;
    try (Connection conn = dataSource.getConnection()) {
      conn.setAutoCommit(false);
      cardLimits.upsertAllLimits(conn, card.id(), perTxnAmount, dailyAmount, dailyCount);
      auditLog.record(
          conn,
          actor(ctx),
          "CARD_LIMITS_UPDATED",
          "card",
          card.cardRef(),
          jsonOf(limitsAsMap(currentLimits)),
          jsonOf(limitsAsMap(newLimits)));
      responseBody = cardDetail(card, newLimits);
      idempotency.store(conn, idempotencyKey, route, requestHash, 200, jsonOf(responseBody));
      conn.commit();
    } catch (java.sql.SQLException e) {
      throw new IllegalStateException("update card limits failed", e);
    }
    ctx.header("ETag", etagFor(newLimits));
    ctx.status(200).json(responseBody);
  }

  private void getLedger(Context ctx) {
    Optional<Card> maybeCard = cards.findByCardRef(ctx.pathParam("cardRef"));
    if (maybeCard.isEmpty()) {
      notFound(ctx, "card");
      return;
    }
    Card card = maybeCard.get();
    int limit = Optional.ofNullable(ctx.queryParam("limit")).map(Integer::parseInt).orElse(50);
    Long cursor = Optional.ofNullable(ctx.queryParam("cursor")).map(Long::parseLong).orElse(null);

    List<JournalEntryRow> rows = ledger.findByAccount(dataSource, card.accountId(), limit, cursor);
    List<Map<String, Object>> items = rows.stream().map(this::journalEntry).toList();
    String nextCursor =
        rows.size() == limit ? String.valueOf(rows.get(rows.size() - 1).journalId()) : null;

    Map<String, Object> body = new LinkedHashMap<>();
    body.put("items", items);
    body.put("nextCursor", nextCursor);
    ctx.json(body);
  }

  // ---- replay / idempotency ----------------------------------------------------------------

  private Optional<IdempotencyRecord> replayIfPresent(
      Context ctx, String route, String key, String requestHash) {
    Optional<IdempotencyRecord> stored = idempotency.find(key, route);
    if (stored.isEmpty()) return Optional.empty();
    IdempotencyRecord record = stored.get();
    if (!record.requestHash().equals(requestHash)) {
      idempotencyKeyMismatch(ctx);
      return stored;
    }
    ctx.status(record.status()).contentType("application/json").result(record.body());
    return stored;
  }

  // ---- response shaping ----------------------------------------------------------------------

  private Map<String, Object> cardSummary(Card card) {
    Map<String, Object> m = new LinkedHashMap<>();
    m.put("cardRef", card.cardRef());
    m.put("maskedPan", maskedPan(card));
    m.put("holderName", card.holderName());
    m.put("status", card.status());
    m.put("expiry", expiryDisplay(card.expiryYymm()));
    return m;
  }

  private Map<String, Object> cardDetail(Card card, List<CardLimit> limits) {
    Map<String, Object> m = cardSummary(card);
    AccountSummary account = accounts.findById(card.accountId()).orElseThrow();
    m.put("ledgerBalance", money(account.ledgerBalance(), account.currency()));
    m.put("availableBalance", money(account.availableBalance(), account.currency()));
    // ponytail: no PREAUTH flow ships yet (MCN-603), so auth_hold is always empty for now.
    m.put("holds", List.of());
    m.put("limits", limitsAsMap(limits, account.currency()));
    m.put("usedToday", money(cardLimits.amountToday(card.id()), account.currency()));
    return m;
  }

  private Map<String, Object> limitsAsMap(List<CardLimit> limits) {
    return limitsAsMap(limits, "704");
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

  private String etagFor(List<CardLimit> limits) {
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
    return '"' + sha256(sb.toString()) + '"';
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

  // ---- RFC 9457 problem responses -------------------------------------------------------------

  private void problem(Context ctx, int status, String suffix, String title, String detail) {
    Map<String, Object> body = new LinkedHashMap<>();
    body.put("type", "https://mcn.local/problems/" + suffix);
    body.put("title", title);
    body.put("status", status);
    body.put("detail", detail);
    body.put("instance", ctx.path());
    body.put("traceId", UUID.randomUUID().toString().replace("-", ""));
    ctx.status(status).contentType("application/problem+json").json(body);
  }

  private void notFound(Context ctx, String entity) {
    problem(
        ctx,
        404,
        "not-found",
        entity + " not found",
        entity + " not found: " + ctx.pathParam("cardRef"));
  }

  private void conflict(Context ctx, String detail) {
    problem(ctx, 409, "conflict", "State transition not allowed", detail);
  }

  private void insufficientIdempotencyKey(Context ctx) {
    problem(
        ctx,
        400,
        "insufficient-idempotency-key",
        "Idempotency-Key header is required",
        ctx.method() + " " + ctx.path() + " requires Idempotency-Key.");
  }

  private void idempotencyKeyMismatch(Context ctx) {
    problem(
        ctx,
        422,
        "idempotency-key-mismatch",
        "Idempotency-Key reused with a different request",
        "The same Idempotency-Key was used with a different request body.");
  }

  private void preconditionFailed(Context ctx) {
    problem(
        ctx,
        412,
        "precondition-failed",
        "ETag mismatch",
        "If-Match does not match the current ETag.");
  }

  private void validationError(Context ctx, String detail) {
    problem(ctx, 400, "validation-error", "Request body invalid", detail);
  }
}
