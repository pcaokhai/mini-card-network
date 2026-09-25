package io.mcn.issuer;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.crypto.CardCrypto;
import io.mcn.issuer.adapter.http.CardAdminController;
import io.mcn.issuer.adapter.http.HealthServer;
import io.mcn.issuer.adapter.http.Readiness;
import io.mcn.issuer.adapter.persistence.AccountRepository;
import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.BusinessDateRepository;
import io.mcn.issuer.adapter.persistence.CardLimitRepository;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.IdempotencyRepository;
import io.mcn.issuer.adapter.persistence.LedgerRepository;
import io.mcn.issuer.adapter.persistence.TestDataSources;
import io.mcn.issuer.adapter.seed.SeedLoader;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import javax.sql.DataSource;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class CardAdminApiIntegrationTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  private final HttpClient client = HttpClient.newHttpClient();
  private HealthServer server;
  private DataSource ds;
  private int port;

  @BeforeEach
  void start() throws Exception {
    ds = TestDataSources.migrated(postgres);
    var crypto =
        new CardCrypto(
            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
            "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210");
    new SeedLoader().load(ds, crypto, Path.of("../contracts/fixtures/cards.json"));

    var cardAdmin =
        new CardAdminController(
            ds,
            new CardRepository(ds),
            new AccountRepository(ds),
            new CardLimitRepository(ds),
            new AuditLogRepository(ds),
            new IdempotencyRepository(ds),
            new LedgerRepository(),
            new BusinessDateRepository(ds));
    server = new HealthServer(new Readiness(), cardAdmin);
    port = server.start(0);
  }

  @AfterEach
  void stop() {
    server.stop();
    ((HikariDataSource) ds).close();
  }

  private HttpResponse<String> get(String path) throws Exception {
    return client.send(
        HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + path)).GET().build(),
        HttpResponse.BodyHandlers.ofString());
  }

  private HttpResponse<String> post(String path, String idempotencyKey, String body)
      throws Exception {
    var builder =
        HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + path))
            .header("Content-Type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(body));
    if (idempotencyKey != null) builder.header("Idempotency-Key", idempotencyKey);
    return client.send(builder.build(), HttpResponse.BodyHandlers.ofString());
  }

  private HttpResponse<String> delete(String path, String idempotencyKey) throws Exception {
    var builder = HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + path)).DELETE();
    if (idempotencyKey != null) builder.header("Idempotency-Key", idempotencyKey);
    return client.send(builder.build(), HttpResponse.BodyHandlers.ofString());
  }

  private HttpResponse<String> put(String path, String idempotencyKey, String ifMatch, String body)
      throws Exception {
    var builder =
        HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + path))
            .header("Content-Type", "application/json")
            .PUT(HttpRequest.BodyPublishers.ofString(body));
    if (idempotencyKey != null) builder.header("Idempotency-Key", idempotencyKey);
    if (ifMatch != null) builder.header("If-Match", ifMatch);
    return client.send(builder.build(), HttpResponse.BodyHandlers.ofString());
  }

  @Test
  void listCardsReturnsSeededFixtureCards() throws Exception {
    var response = get("/v1/cards");
    assertThat(response.statusCode()).isEqualTo(200);
    assertThat(response.body()).contains("970436").contains("4417").contains("crd_normal0001");
    assertThat(response.body()).doesNotContain("9704360000004417"); // never the full PAN
  }

  @Test
  void getCardReturnsAnETagHeader() throws Exception {
    var response = get("/v1/cards/crd_normal0001");
    assertThat(response.statusCode()).isEqualTo(200);
    assertThat(response.headers().firstValue("ETag")).isPresent();
    assertThat(response.body()).contains("Nguyen Minh Anh").contains("ledgerBalance");
  }

  @Test
  void getUnknownCardReturns404NotFound() throws Exception {
    var response = get("/v1/cards/crd_doesnotexist01");
    assertThat(response.statusCode()).isEqualTo(404);
    assertThat(response.body()).contains("https://mcn.local/problems/not-found");
  }

  @Test
  void blockCardRequiresIdempotencyKey() throws Exception {
    var response =
        post("/v1/cards/crd_normal0001/blocks", null, "{\"reason\":\"CUSTOMER_REQUEST\"}");
    assertThat(response.statusCode()).isEqualTo(400);
    assertThat(response.body()).contains("https://mcn.local/problems/insufficient-idempotency-key");
  }

  @Test
  void blockCardWritesAnAuditLogEntry() throws Exception {
    var auditRepo = new AuditLogRepository(ds);
    var response =
        post(
            "/v1/cards/crd_normal0001/blocks",
            UUID.randomUUID().toString(),
            "{\"reason\":\"LOST\"}");
    assertThat(response.statusCode()).isEqualTo(200);
    assertThat(response.body()).contains("\"status\":\"LOST\"");

    var entries = auditRepo.findByEntity("card", "crd_normal0001");
    assertThat(entries).hasSize(1);
    assertThat(entries.get(0).action()).isEqualTo("CARD_BLOCKED");
  }

  @Test
  void replayingTheSameIdempotencyKeyReturnsTheStoredResponseUnchanged() throws Exception {
    String key = UUID.randomUUID().toString();
    String body = "{\"reason\":\"LOST\"}";
    var first = post("/v1/cards/crd_lowbal0002/blocks", key, body);
    var second = post("/v1/cards/crd_lowbal0002/blocks", key, body);
    assertThat(first.statusCode()).isEqualTo(200);
    assertThat(second.statusCode()).isEqualTo(200);
    // replayed response is the stored one, re-serialized by a different ObjectMapper instance -
    // compare parsed JSON, not raw bytes, since key order isn't part of the "unchanged" contract.
    var mapper = new ObjectMapper();
    assertThat(mapper.readTree(second.body())).isEqualTo(mapper.readTree(first.body()));

    var auditRepo = new AuditLogRepository(ds);
    assertThat(auditRepo.findByEntity("card", "crd_lowbal0002")).hasSize(1);
  }

  @Test
  void reusingTheSameIdempotencyKeyWithADifferentBodyReturns422() throws Exception {
    String key = UUID.randomUUID().toString();
    String cardRef = newCard("ACTIVE", "3012");
    post("/v1/cards/" + cardRef + "/blocks", key, "{\"reason\":\"LOST\"}");
    var mismatched = post("/v1/cards/" + cardRef + "/blocks", key, "{\"reason\":\"STOLEN\"}");
    assertThat(mismatched.statusCode()).isEqualTo(422);
    assertThat(mismatched.body()).contains("https://mcn.local/problems/idempotency-key-mismatch");
  }

  @Test
  void unblockingAPreblockedCardSucceedsThenConflictsOnRetryFromActive() throws Exception {
    // crd_blockd0003 seeds as BLOCKED (contracts/fixtures/cards.json) - self-contained: this test
    // never depends on another test's mutation of a shared fixture card.
    var unblocked = delete("/v1/cards/crd_blockd0003/blocks", UUID.randomUUID().toString());
    assertThat(unblocked.statusCode()).isEqualTo(200);
    assertThat(unblocked.body()).contains("\"status\":\"ACTIVE\"");

    var conflict = delete("/v1/cards/crd_blockd0003/blocks", UUID.randomUUID().toString());
    assertThat(conflict.statusCode()).isEqualTo(409);
    assertThat(conflict.body()).contains("https://mcn.local/problems/conflict");
  }

  @Test
  void updateLimitsRequiresIfMatchAndReturns412OnStaleEtag() throws Exception {
    var getResponse = get("/v1/cards/crd_normal0001");
    String realEtag = getResponse.headers().firstValue("ETag").orElseThrow();

    String body =
        "{\"dailyAmount\":{\"amount\":2000000,\"currency\":\"704\"},"
            + "\"perTransactionAmount\":{\"amount\":500000,\"currency\":\"704\"},\"dailyCount\":10}";
    var stale =
        put(
            "/v1/cards/crd_normal0001/limits",
            UUID.randomUUID().toString(),
            "\"wrong-etag\"",
            body);
    assertThat(stale.statusCode()).isEqualTo(412);
    assertThat(stale.body()).contains("https://mcn.local/problems/precondition-failed");

    var ok = put("/v1/cards/crd_normal0001/limits", UUID.randomUUID().toString(), realEtag, body);
    assertThat(ok.statusCode()).isEqualTo(200);
    assertThat(ok.body()).contains("\"perTransactionAmount\"");
  }

  @Test
  void getCardLedgerReturnsEmptyItemsWhenNoPurchasesYet() throws Exception {
    var response = get("/v1/cards/crd_normal0001/ledger");
    assertThat(response.statusCode()).isEqualTo(200);
    assertThat(response.body()).contains("\"items\":[]").contains("\"nextCursor\":null");
  }

  // ---- CARDS-G* gap fixes (docs/api/cards-page.md §9) ----------------------------------------

  private static final ObjectMapper JSON = new ObjectMapper();

  private HttpResponse<String> send(String method, String path, String body, String... headers)
      throws Exception {
    var builder =
        HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + path))
            .method(
                method,
                body == null
                    ? HttpRequest.BodyPublishers.noBody()
                    : HttpRequest.BodyPublishers.ofString(body));
    if (body != null) builder.header("Content-Type", "application/json");
    if (headers.length > 0) builder.headers(headers);
    return client.send(builder.build(), HttpResponse.BodyHandlers.ofString());
  }

  private HttpResponse<String> block(String cardRef, String reason, String actor) throws Exception {
    return send(
        "POST",
        "/v1/cards/" + cardRef + "/blocks",
        "{\"reason\":\"" + reason + "\"}",
        "Idempotency-Key",
        UUID.randomUUID().toString(),
        "X-Actor",
        actor);
  }

  private HttpResponse<String> unblock(String cardRef, String actor) throws Exception {
    return send(
        "DELETE",
        "/v1/cards/" + cardRef + "/blocks",
        null,
        "Idempotency-Key",
        UUID.randomUUID().toString(),
        "X-Actor",
        actor);
  }

  private static String limitsBody(long perTxn, long daily, String currency, Integer dailyCount) {
    return "{\"dailyAmount\":{\"amount\":"
        + daily
        + ",\"currency\":\""
        + currency
        + "\"},\"perTransactionAmount\":{\"amount\":"
        + perTxn
        + ",\"currency\":\""
        + currency
        + "\"},\"dailyCount\":"
        + dailyCount
        + "}";
  }

  private String etagOf(String cardRef) throws Exception {
    return get("/v1/cards/" + cardRef).headers().firstValue("ETag").orElseThrow();
  }

  /** A fresh card of its own per test: the container DB is shared by every test in this class. */
  private String newCard(String status, String expiryYymm) throws Exception {
    String cardRef = "crd_t" + UUID.randomUUID().toString().replace("-", "").substring(0, 11);
    try (var conn = ds.getConnection();
        var account =
            conn.prepareStatement(
                "INSERT INTO account (account_no, currency, ledger_balance, available_balance)"
                    + " VALUES (?, '704', 1000000, 1000000) RETURNING id");
        var card =
            conn.prepareStatement(
                "INSERT INTO card (account_id, pan_enc, pan_hash, bin, pan_last4, expiry_yymm,"
                    + " status, card_ref, holder_name)"
                    + " VALUES (?, ?, ?, '970436', '9999', ?, ?, ?, 'Test Holder')")) {
      account.setString(1, "ACC-" + cardRef);
      var rs = account.executeQuery();
      rs.next();
      card.setLong(1, rs.getLong(1));
      card.setBytes(2, UUID.randomUUID().toString().getBytes());
      card.setBytes(3, UUID.randomUUID().toString().getBytes());
      card.setString(4, expiryYymm);
      card.setString(5, status);
      card.setString(6, cardRef);
      card.executeUpdate();
    }
    return cardRef;
  }

  private void execute(String sql) throws Exception {
    try (var conn = ds.getConnection();
        var stmt = conn.createStatement()) {
      stmt.execute(sql);
    }
  }

  private String storedStatus(String cardRef) throws Exception {
    try (var conn = ds.getConnection();
        var stmt = conn.prepareStatement("SELECT status FROM card WHERE card_ref = ?")) {
      stmt.setString(1, cardRef);
      var rs = stmt.executeQuery();
      rs.next();
      return rs.getString(1);
    }
  }

  @Test
  @DisplayName("CARDS-G1: card, ledger and audit reads are never cacheable")
  void should_sendNoStore_when_readingCardsLedgerOrAudit() throws Exception {
    for (String path :
        List.of(
            "/v1/cards",
            "/v1/cards/crd_normal0001",
            "/v1/cards/crd_normal0001/ledger",
            "/v1/cards/crd_normal0001/audit")) {
      assertThat(get(path).headers().firstValue("Cache-Control")).as(path).contains("no-store");
    }
  }

  @Test
  @DisplayName("CARDS-G1: If-None-Match with an ETag from before a status change is not a 304")
  void should_return200_when_ifNoneMatchIsFromBeforeAStatusChange() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    String etag = etagOf(cardRef);
    block(cardRef, "CUSTOMER_REQUEST", "ops");

    var response = send("GET", "/v1/cards/" + cardRef, null, "If-None-Match", etag);

    assertThat(response.statusCode()).isEqualTo(200);
    assertThat(response.body()).contains("\"status\":\"BLOCKED\"");
  }

  @Test
  @DisplayName("CARDS-G1: the ETag versions the limits, so a status change doesn't fail If-Match")
  void should_acceptTheETagAsIfMatch_when_onlyTheStatusChanged() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    String etag = etagOf(cardRef);
    block(cardRef, "CUSTOMER_REQUEST", "ops");

    var response =
        put(
            "/v1/cards/" + cardRef + "/limits",
            UUID.randomUUID().toString(),
            etag,
            limitsBody(100_000, 900_000, "704", 5));

    assertThat(response.statusCode()).isEqualTo(200);
  }

  @Test
  @DisplayName("CARDS-G2: audit lists admin actions newest first with the stored actor")
  void should_listAuditNewestFirstWithActor_when_cardHasAdminActions() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    block(cardRef, "FRAUD_SUSPECTED", "ops-alice");
    unblock(cardRef, "ops-bob");

    var response = get("/v1/cards/" + cardRef + "/audit");

    assertThat(response.statusCode()).isEqualTo(200);
    var items = JSON.readTree(response.body()).path("items");
    assertThat(items).hasSize(2);
    assertThat(items.get(0).path("action").asText()).isEqualTo("CARD_UNBLOCKED");
    assertThat(items.get(0).path("actor").asText()).isEqualTo("ops-bob");
    assertThat(items.get(1).path("action").asText()).isEqualTo("CARD_BLOCKED");
    assertThat(items.get(1).path("actor").asText()).isEqualTo("ops-alice");
    assertThat(items.get(1).path("before").path("status").asText()).isEqualTo("ACTIVE");
    assertThat(items.get(1).path("after").path("reason").asText()).isEqualTo("FRAUD_SUSPECTED");
    assertThat(items.get(1).path("auditId").isTextual()).isTrue();
    assertThat(items.get(1).path("occurredAt").asText()).isNotBlank();
  }

  @Test
  @DisplayName("CARDS-G2: audit pages with an opaque cursor")
  void should_pageAudit_when_moreEntriesThanTheLimit() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    block(cardRef, "CUSTOMER_REQUEST", "ops");
    unblock(cardRef, "ops");
    block(cardRef, "CUSTOMER_REQUEST", "ops");

    var first = JSON.readTree(get("/v1/cards/" + cardRef + "/audit?limit=2").body());
    String cursor = first.path("nextCursor").asText();
    var second =
        JSON.readTree(get("/v1/cards/" + cardRef + "/audit?limit=2&cursor=" + cursor).body());

    assertThat(first.path("items")).hasSize(2);
    assertThat(second.path("items")).hasSize(1);
    assertThat(second.path("items").get(0).path("action").asText()).isEqualTo("CARD_BLOCKED");
    assertThat(second.path("nextCursor").isNull()).isTrue();
  }

  @Test
  @DisplayName("CARDS-G2: audit of an unknown card is 404")
  void should_return404_when_auditingAnUnknownCard() throws Exception {
    assertThat(get("/v1/cards/crd_doesnotexist01/audit").statusCode()).isEqualTo(404);
  }

  @Test
  @DisplayName("CARDS-G4: a card past its expiry month reads EXPIRED without mutating the row")
  void should_readExpired_when_pastTheExpiryMonth() throws Exception {
    String cardRef = newCard("ACTIVE", "2001");

    assertThat(get("/v1/cards/" + cardRef).body()).contains("\"status\":\"EXPIRED\"");
    assertThat(get("/v1/cards").body())
        .contains("\"cardRef\":\"" + cardRef + "\"")
        .contains("\"status\":\"EXPIRED\"");
    assertThat(storedStatus(cardRef)).isEqualTo("ACTIVE");
  }

  @Test
  @DisplayName("CARDS-G5: a reason outside the enum is a 400 validation-error")
  void should_return400_when_blockReasonIsNotInTheEnum() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");

    var response = block(cardRef, "BORED", "ops");

    assertThat(response.statusCode()).isEqualTo(400);
    var problem = JSON.readTree(response.body());
    assertThat(problem.path("type").asText())
        .isEqualTo("https://mcn.local/problems/validation-error");
    assertThat(problem.path("errors").get(0).path("field").asText()).isEqualTo("reason");
    assertThat(storedStatus(cardRef)).isEqualTo("ACTIVE");
  }

  @Test
  @DisplayName("CARDS-G5: the block reason picks the status and is kept in the audit row")
  void should_mapReasonToStatus_when_blocking() throws Exception {
    var expected =
        Map.of(
            "LOST", "LOST",
            "STOLEN", "STOLEN",
            "FRAUD_SUSPECTED", "BLOCKED",
            "CUSTOMER_REQUEST", "BLOCKED");
    for (var entry : expected.entrySet()) {
      String cardRef = newCard("ACTIVE", "3012");
      var response = block(cardRef, entry.getKey(), "ops");
      assertThat(response.statusCode()).as(entry.getKey()).isEqualTo(200);
      assertThat(storedStatus(cardRef)).as(entry.getKey()).isEqualTo(entry.getValue());
      var audit = new AuditLogRepository(ds).findByEntity("card", cardRef).get(0);
      assertThat(JSON.readTree(audit.afterState()).path("reason").asText())
          .isEqualTo(entry.getKey());
    }
  }

  @Test
  @DisplayName("CARDS-G5: only a BLOCKED card can be unblocked")
  void should_return409_when_unblockingLostStolenPinBlockedOrExpired() throws Exception {
    for (String cardRef :
        List.of(
            newCard("LOST", "3012"),
            newCard("STOLEN", "3012"),
            newCard("PIN_BLOCKED", "3012"),
            newCard("BLOCKED", "2001"))) {
      var response = unblock(cardRef, "ops");
      assertThat(response.statusCode()).isEqualTo(409);
      assertThat(response.body()).contains("https://mcn.local/problems/conflict");
    }
  }

  @Test
  @DisplayName("CARDS-G6: a retried successful PUT replays even though its If-Match is now stale")
  void should_replay_when_retryingASuccessfulLimitsPut() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    String etag = etagOf(cardRef);
    String key = UUID.randomUUID().toString();
    String body = limitsBody(100_000, 900_000, "704", 5);

    var first = put("/v1/cards/" + cardRef + "/limits", key, etag, body);
    var retry = put("/v1/cards/" + cardRef + "/limits", key, etag, body);

    assertThat(first.statusCode()).isEqualTo(200);
    assertThat(retry.statusCode()).isEqualTo(200);
    assertThat(JSON.readTree(retry.body())).isEqualTo(JSON.readTree(first.body()));
  }

  @Test
  @DisplayName("CARDS-G6: concurrent PUTs with the same ETag - exactly one wins, the rest get 412")
  void should_letExactlyOneWin_when_concurrentPutsShareAnETag() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    String etag = etagOf(cardRef);
    int writers = 8;
    var start = new CountDownLatch(1);
    List<Future<Integer>> statuses = new ArrayList<>();
    try (var pool = Executors.newFixedThreadPool(writers)) {
      for (int i = 0; i < writers; i++) {
        String body = limitsBody(100_000, 1_000_000 + i, "704", 5);
        statuses.add(
            pool.submit(
                () -> {
                  start.await();
                  return put(
                          "/v1/cards/" + cardRef + "/limits",
                          UUID.randomUUID().toString(),
                          etag,
                          body)
                      .statusCode();
                }));
      }
      start.countDown();
      List<Integer> results = new ArrayList<>();
      for (var f : statuses) results.add(f.get(30, TimeUnit.SECONDS));
      assertThat(results).containsOnly(200, 412);
      assertThat(results.stream().filter(s -> s == 200).count()).isEqualTo(1);
    }
    assertThat(new AuditLogRepository(ds).findByEntity("card", cardRef)).hasSize(1);
  }

  @Test
  @DisplayName("CARDS-G6: writers queued on the card lock can't starve the lock holder's pool")
  void should_notStarveThePool_when_moreWritersThanConnectionsQueueOnOneCard() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    var cfg = new com.zaxxer.hikari.HikariConfig();
    cfg.setJdbcUrl(postgres.getJdbcUrl());
    cfg.setUsername(postgres.getUsername());
    cfg.setPassword(postgres.getPassword());
    cfg.setMaximumPoolSize(2);
    cfg.setConnectionTimeout(2_000);
    try (var tinyPool = new HikariDataSource(cfg)) {
      var tinyServer =
          new HealthServer(
              new Readiness(),
              new CardAdminController(
                  tinyPool,
                  new CardRepository(tinyPool),
                  new AccountRepository(tinyPool),
                  new CardLimitRepository(tinyPool),
                  new AuditLogRepository(tinyPool),
                  new IdempotencyRepository(tinyPool),
                  new LedgerRepository(),
                  new BusinessDateRepository(tinyPool)));
      int tinyPort = tinyServer.start(0);
      try {
        String etag = etagOf(cardRef);
        var start = new CountDownLatch(1);
        List<Future<Integer>> statuses = new ArrayList<>();
        try (var pool = Executors.newFixedThreadPool(6)) {
          for (int i = 0; i < 6; i++) {
            String body = limitsBody(100_000, 2_000_000 + i, "704", 5);
            statuses.add(
                pool.submit(
                    () -> {
                      start.await();
                      var request =
                          HttpRequest.newBuilder(
                                  URI.create(
                                      "http://127.0.0.1:"
                                          + tinyPort
                                          + "/v1/cards/"
                                          + cardRef
                                          + "/limits"))
                              .header("Content-Type", "application/json")
                              .header("Idempotency-Key", UUID.randomUUID().toString())
                              .header("If-Match", etag)
                              .PUT(HttpRequest.BodyPublishers.ofString(body))
                              .build();
                      return client
                          .send(request, HttpResponse.BodyHandlers.ofString())
                          .statusCode();
                    }));
          }
          start.countDown();
          List<Integer> results = new ArrayList<>();
          for (var f : statuses) results.add(f.get(60, TimeUnit.SECONDS));
          assertThat(results).containsOnly(200, 412);
          assertThat(results.stream().filter(s -> s == 200).count()).isEqualTo(1);
        }
      } finally {
        tinyServer.stop();
      }
    }
  }

  @Test
  @DisplayName("CARDS-G7: invalid limits give a validation-error with one errors[] entry per field")
  void should_return400WithFieldErrors_when_limitsAreInvalid() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    String etag = etagOf(cardRef);

    var badFields =
        put(
            "/v1/cards/" + cardRef + "/limits",
            UUID.randomUUID().toString(),
            etag,
            "{\"dailyAmount\":{\"amount\":-1,\"currency\":\"704\"},"
                + "\"perTransactionAmount\":{\"amount\":100,\"currency\":\"840\"},"
                + "\"dailyCount\":-2}");
    var perTxnOverDaily =
        put(
            "/v1/cards/" + cardRef + "/limits",
            UUID.randomUUID().toString(),
            etag,
            limitsBody(900_000, 100_000, "704", null));

    assertThat(badFields.statusCode()).isEqualTo(400);
    assertThat(JSON.readTree(badFields.body()).path("errors").findValuesAsText("field"))
        .containsExactlyInAnyOrder(
            "dailyAmount.amount", "perTransactionAmount.currency", "dailyCount");
    assertThat(perTxnOverDaily.statusCode()).isEqualTo(400);
    assertThat(JSON.readTree(perTxnOverDaily.body()).path("errors").findValuesAsText("field"))
        .containsExactly("perTransactionAmount.amount");
  }

  @Test
  @DisplayName("CARDS-G7: a zero ceiling is a validation-error, not a 500 from the DB check")
  void should_return400_when_limitsAreZero() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");

    var response =
        put(
            "/v1/cards/" + cardRef + "/limits",
            UUID.randomUUID().toString(),
            etagOf(cardRef),
            limitsBody(0, 0, "704", 0));

    assertThat(response.statusCode()).isEqualTo(400);
    assertThat(JSON.readTree(response.body()).path("errors").findValuesAsText("field"))
        .containsExactlyInAnyOrder(
            "perTransactionAmount.amount", "dailyAmount.amount", "dailyCount");
  }

  @Test
  @DisplayName("CARDS-G8: ledger limit must be 1-200 and cursor an integer")
  void should_return400_when_ledgerPagingIsInvalid() throws Exception {
    for (String query : List.of("limit=abc", "limit=0", "limit=201", "cursor=abc", "cursor=-1")) {
      var response = get("/v1/cards/crd_normal0001/ledger?" + query);
      assertThat(response.statusCode()).as(query).isEqualTo(400);
      assertThat(response.body()).as(query).contains("validation-error").contains("\"errors\"");
    }
    assertThat(get("/v1/cards/crd_normal0001/ledger?limit=200").statusCode()).isEqualTo(200);
  }

  @Test
  @DisplayName("CARDS-G9: problems are problem+json and carry the request's trace id")
  void should_useTheTraceparentTraceId_when_returningAProblem() throws Exception {
    String traceId = "4bf92f3577b34da6a3ce929d0e0e4736";

    var response =
        send(
            "GET",
            "/v1/cards/crd_doesnotexist01",
            null,
            "traceparent",
            "00-" + traceId + "-00f067aa0ba902b7-01");

    assertThat(response.headers().firstValue("Content-Type"))
        .hasValueSatisfying(ct -> assertThat(ct).startsWith("application/problem+json"));
    assertThat(response.headers().firstValue("X-Trace-Id")).contains(traceId);
    var problem = JSON.readTree(response.body());
    assertThat(problem.path("traceId").asText()).isEqualTo(traceId);
    assertThat(problem.path("instance").asText()).isEqualTo("/v1/cards/crd_doesnotexist01");
  }

  @Test
  @DisplayName("CARDS-G9: without traceparent the problem's traceId matches X-Trace-Id")
  void should_generateATraceId_when_noTraceparentIsSent() throws Exception {
    var response = get("/v1/cards/crd_doesnotexist01");

    String header = response.headers().firstValue("X-Trace-Id").orElseThrow();
    assertThat(header).matches("[0-9a-f]{32}");
    assertThat(JSON.readTree(response.body()).path("traceId").asText()).isEqualTo(header);
  }

  @Test
  @DisplayName("CARDS-G10: an Idempotency-Key older than 24 h is a new request, and is purged")
  void should_processAsNew_when_theIdempotencyRecordIsOlderThan24h() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    String key = UUID.randomUUID().toString();
    put(
        "/v1/cards/" + cardRef + "/limits",
        key,
        etagOf(cardRef),
        limitsBody(100_000, 900_000, "704", 5));
    execute("UPDATE idempotency_record SET created_at = now() - interval '25 hours'");

    var reused =
        put(
            "/v1/cards/" + cardRef + "/limits",
            key,
            etagOf(cardRef),
            limitsBody(200_000, 800_000, "704", 5));

    assertThat(reused.statusCode()).isEqualTo(200);
    assertThat(reused.body()).contains("\"amount\":200000");
    try (var conn = ds.getConnection();
        var stmt =
            conn.prepareStatement(
                "SELECT count(*) FROM idempotency_record"
                    + " WHERE created_at < now() - interval '24 hours'")) {
      var rs = stmt.executeQuery();
      rs.next();
      assertThat(rs.getLong(1)).isZero();
    }
  }

  @Test
  @DisplayName("CARDS-G12: usedToday sums the velocity counter of the system_state business date")
  void should_useTheBusinessDate_when_computingUsedToday() throws Exception {
    String cardRef = newCard("ACTIVE", "3012");
    String cardId = "(SELECT id FROM card WHERE card_ref = '" + cardRef + "')";
    execute(
        "INSERT INTO velocity_counter (card_id, tran_type, period, period_key, txn_count,"
            + " txn_amount) VALUES ("
            + cardId
            + ", 'PURCHASE', 'DAILY', '2026-01-15', 1, 12345), ("
            + cardId
            + ", 'PURCHASE', 'DAILY', '2026-01-16', 1, 999)");
    execute("INSERT INTO system_state (id, current_business_date) VALUES (1, '2026-01-15')");
    try {
      var body = JSON.readTree(get("/v1/cards/" + cardRef).body());
      assertThat(body.path("usedToday").path("amount").asLong()).isEqualTo(12345);
    } finally {
      execute("DELETE FROM system_state");
    }
  }

  @Test
  @DisplayName("CARDS-G13: a cardRef outside the contract pattern is a 400")
  void should_return400_when_cardRefDoesNotMatchThePattern() throws Exception {
    for (String path :
        List.of("/v1/cards/crd_does_not_exist", "/v1/cards/nope", "/v1/cards/crd_short/ledger")) {
      var response = get(path);
      assertThat(response.statusCode()).as(path).isEqualTo(400);
      assertThat(response.body()).as(path).contains("validation-error");
    }
  }
}
