package io.mcn.issuer.adapter.http;

import static org.assertj.core.api.Assertions.assertThat;

import io.mcn.issuer.adapter.persistence.KeyStoreRow;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Instant;
import java.util.List;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

class KeysEndpointTest {
  private final HttpClient client = HttpClient.newHttpClient();
  private final Readiness readiness = new Readiness();
  private final KeysController keysController =
      new KeysController(
          () ->
              List.of(
                  new KeyStoreRow(
                      1,
                      "ZPK",
                      "970436000",
                      "aa".repeat(16),
                      "AABBCC",
                      "ACTIVE",
                      Instant.now(),
                      null,
                      Instant.now())));
  private final HealthServer server = new HealthServer(readiness, null, keysController);

  @AfterEach
  void stop() {
    server.stop();
  }

  @Test
  void should_return_key_info_array_with_no_clear_key_field__MCN_501_AC1_AC2() throws Exception {
    int port = server.start("127.0.0.1", 0);

    HttpResponse<String> resp =
        client.send(
            HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + "/v1/keys/issuer"))
                .build(),
            HttpResponse.BodyHandlers.ofString());

    assertThat(resp.statusCode()).isEqualTo(200);
    assertThat(resp.body())
        .contains("\"kcv\"")
        .contains("\"daysRemaining\"")
        .doesNotContain("keyUnderLmk")
        .doesNotContain("key_under_lmk");
  }
}
