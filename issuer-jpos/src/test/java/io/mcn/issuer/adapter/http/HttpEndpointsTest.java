package io.mcn.issuer.adapter.http;

import static org.assertj.core.api.Assertions.assertThat;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

class HttpEndpointsTest {
  private final HttpClient client = HttpClient.newHttpClient();
  private final Readiness readiness = new Readiness();
  private final HealthServer server = new HealthServer(readiness);

  @AfterEach
  void stop() {
    server.stop();
  }

  private HttpResponse<String> get(int port, String path) throws Exception {
    return client.send(
        HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + path)).build(),
        HttpResponse.BodyHandlers.ofString());
  }

  @Test
  void should_report_live_and_ready__MCN_005_AC1() throws Exception {
    int port = server.start(0);
    assertThat(get(port, "/health/live").statusCode()).isEqualTo(200);
    assertThat(get(port, "/health/ready").body()).isEqualTo("{\"status\":\"UP\"}");
  }

  @Test
  void should_turn_not_ready_while_draining__MCN_005_AC4() throws Exception {
    int port = server.start(0);
    readiness.startDraining();
    var ready = get(port, "/health/ready");
    assertThat(ready.statusCode()).isEqualTo(503);
    assertThat(ready.body()).isEqualTo("{\"status\":\"DRAINING\"}");
  }
}
