package io.mcn.settlement;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.micrometer.metrics.test.autoconfigure.AutoConfigureMetrics;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.web.client.RestClient;

// Spring Boot 4 disables metrics export under @SpringBootTest by default (a
// MetricsContextCustomizerFactory.DisableMetricsExportContextCustomizer runs unless this
// annotation is present) - without it, /prometheus 404s even though the endpoint is exposed.
@AutoConfigureMetrics
@SpringBootTest(
    webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
    properties = "management.server.port=0")
class HealthEndpointsTest {

  @Value("${local.management.port}")
  int managementPort;

  private String get(String path) {
    return RestClient.create("http://127.0.0.1:" + managementPort)
        .get()
        .uri(path)
        .retrieve()
        .body(String.class);
  }

  @Test
  void should_expose_live_and_ready_on_management_port__MCN_005_AC1() {
    assertThat(get("/health/live")).contains("\"status\":\"UP\"");
    assertThat(get("/health/ready")).contains("\"status\":\"UP\"");
  }

  @Test
  void should_expose_prometheus_metrics__MCN_005_AC1() {
    assertThat(get("/prometheus")).contains("jvm_memory");
  }
}
