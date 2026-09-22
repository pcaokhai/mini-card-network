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
import java.util.UUID;
import javax.sql.DataSource;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
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
            new LedgerRepository());
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
    var response = get("/v1/cards/crd_does_not_exist");
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
    var auditRepo = new AuditLogRepository(TestDataSources.migrated(postgres));
    var response =
        post(
            "/v1/cards/crd_normal0001/blocks",
            UUID.randomUUID().toString(),
            "{\"reason\":\"LOST\"}");
    assertThat(response.statusCode()).isEqualTo(200);
    assertThat(response.body()).contains("\"status\":\"BLOCKED\"");

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

    var auditRepo = new AuditLogRepository(TestDataSources.migrated(postgres));
    assertThat(auditRepo.findByEntity("card", "crd_lowbal0002")).hasSize(1);
  }

  @Test
  void reusingTheSameIdempotencyKeyWithADifferentBodyReturns422() throws Exception {
    String key = UUID.randomUUID().toString();
    post("/v1/cards/crd_expird0004/blocks", key, "{\"reason\":\"LOST\"}");
    var mismatched = post("/v1/cards/crd_expird0004/blocks", key, "{\"reason\":\"STOLEN\"}");
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
}
