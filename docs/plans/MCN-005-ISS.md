# MCN-005-ISS Issuer service skeleton — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development with superpowers:test-driven-development. Worktree `../mcn-worktrees/iss-005`, branch `feat/MCN-005-iss-skeleton`. Requires MCN-002 merged. Parallel with MCN-005-GW and MCN-005-SET.

**Goal:** `issuer-jpos` starts Q2 with an HTTP QBean serving `/health/live` and `/health/ready` on :8081 and Prometheus metrics on :9465, logs JSON with PAN masking and trace ids, is traced by the OpenTelemetry Java agent, and stops gracefully on SIGTERM.

**Architecture:** Plain jPOS Q2 application (`org.jpos.q2.Q2` main) with deploy descriptors in `src/dist/deploy`. Two QBeans: `HttpEndpoints` (Javalin, health) and `MetricsEndpoint` (Javalin, Micrometer Prometheus scrape). SLF4J + Logback + logstash-logback-encoder with a masking JSON generator decorator. Tracing through the OpenTelemetry Java agent (no tracing code).

**Tech stack:** Java 25, Gradle (Kotlin DSL, wrapper obtained from Spring Initializr), jPOS, Javalin, Micrometer Prometheus registry, Logback, logstash-logback-encoder, JUnit 5, AssertJ, ArchUnit, Spotless. Library versions: `latest.release` in the version catalog, then **locked** with Gradle dependency locking.

## Understanding

MCN-005 AC (docs/06 §E0), issuer rules in `issuer-jpos/CLAUDE.md`, ports docs/02 §8 (8000 ISO from MCN-201, 8081 admin/health, 9465 metrics).

**Not executed in the planning sandbox** (no access to Maven Central there). Run every verification step and fix with superpowers:systematic-debugging before moving on. APIs most likely to differ by version are flagged "check" below.

## Rulings

- Versions: `latest.release` + `./gradlew dependencies --write-locks` produces `gradle.lockfile`; builds are then reproducible and upgrades are explicit PRs. Plugin versions cannot be dynamic in the `plugins {}` block, so Task 1 looks them up once and writes them to `gradle.properties`.
- jPOS internal log events keep jPOS's own listener in Sprint 0; MCN-102 bridges them into the JSON pipeline together with `IsoLogFormatter` (after confirming which jPOS log listener API the resolved version offers). Application logs (our code, Javalin) are JSON from day one.
- The Java masker is duplicated in settlement (no shared business library, docs/10 §2).
- AC3 verified with a `traceparent` sent directly to the service (same ruling as MCN-005-GW).

## File map

| Action | Path (under `issuer-jpos/`) |
| --- | --- |
| Create | Gradle wrapper, `settings.gradle.kts`, `build.gradle.kts`, `gradle.properties`, `gradle/libs.versions.toml`, `gradle.lockfile`, `Makefile` |
| Create | `src/main/java/io/mcn/issuer/adapter/http/HttpEndpoints.java`, `HealthServer.java`, `MetricsEndpoint.java`, `Readiness.java` |
| Create | `src/main/java/io/mcn/issuer/adapter/logging/PanMasker.java`, `src/main/resources/logback.xml` |
| Create | `src/dist/deploy/00_logger.xml`, `40_http_endpoints.xml`, `45_metrics_endpoint.xml` |
| Create | tests: `PanMaskerTest`, `LogbackMaskingTest`, `HttpEndpointsTest`, `ArchitectureTest` |
| Create | `Dockerfile`, `compose.yaml` |

---

### Task 1: Gradle project with wrapper and locked versions

```bash
curl -fsS https://start.spring.io/starter.tgz -d type=gradle-project-kotlin -d language=java -d javaVersion=21 \
  -d groupId=io.mcn -d artifactId=issuer -d baseDir=issuer-jpos-scaffold | tar -xzf -
mkdir -p issuer-jpos && cp -r issuer-jpos-scaffold/{gradlew,gradlew.bat,gradle} issuer-jpos/ && rm -rf issuer-jpos-scaffold
cd issuer-jpos
# Look up current plugin versions once (record them in docs/02 §9):
#   https://plugins.gradle.org/plugin/com.diffplug.spotless
echo "spotlessVersion=<value shown on plugins.gradle.org>" > gradle.properties
```

The `<value …>` above is looked up at execution, not guessed; the PR must contain a concrete version.

`settings.gradle.kts`:

```kotlin
rootProject.name = "issuer-jpos"
pluginManagement {
    val spotlessVersion: String by settings
    plugins { id("com.diffplug.spotless") version spotlessVersion }
}
```

`gradle/libs.versions.toml`:

```toml
[libraries]
jpos = { module = "org.jpos:jpos", version = "latest.release" }
javalin = { module = "io.javalin:javalin", version = "latest.release" }
micrometer-prometheus = { module = "io.micrometer:micrometer-registry-prometheus", version = "latest.release" }
logback = { module = "ch.qos.logback:logback-classic", version = "latest.release" }
logstash-encoder = { module = "net.logstash.logback:logstash-logback-encoder", version = "latest.release" }
junit-bom = { module = "org.junit:junit-bom", version = "latest.release" }
assertj = { module = "org.assertj:assertj-core", version = "latest.release" }
archunit = { module = "com.tngtech.archunit:archunit-junit5", version = "latest.release" }
```

`build.gradle.kts`:

```kotlin
plugins {
    application
    id("com.diffplug.spotless")
}

java { toolchain { languageVersion = JavaLanguageVersion.of(21) } }
repositories { mavenCentral() }
dependencyLocking { lockAllConfigurations() }

dependencies {
    implementation(libs.jpos)
    implementation(libs.javalin)
    implementation(libs.micrometer.prometheus)
    implementation(libs.logback)
    implementation(libs.logstash.encoder)
    testImplementation(platform(libs.junit.bom))
    testImplementation("org.junit.jupiter:junit-jupiter")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
    testImplementation(libs.assertj)
    testImplementation(libs.archunit)
}

application { mainClass = "org.jpos.q2.Q2" }
tasks.named<JavaExec>("run") { workingDir = file("src/dist") }
distributions { main { contents { from("src/dist") } } }
tasks.test { useJUnitPlatform() }
spotless { java { googleJavaFormat(); target("src/**/*.java") } }
```

`Makefile`:

```makefile
SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.PHONY: test lint fmt seed run
test: ; ./gradlew --no-daemon test
lint: ; ./gradlew --no-daemon spotlessCheck
fmt:  ; ./gradlew --no-daemon spotlessApply
seed: ; @echo "issuer seed arrives with MCN-301"
run:  ; ./gradlew --no-daemon run
```

Run: `./gradlew dependencies --write-locks && ./gradlew build` → BUILD SUCCESSFUL (no sources yet). Record resolved versions from `gradle.lockfile` in docs/02 §9. Commit: `build(iss): gradle project with locked dependencies (MCN-005)`.

### Task 2: PAN masker (AC2)

**Step 1 — failing test** `src/test/java/io/mcn/issuer/adapter/logging/PanMaskerTest.java`

```java
package io.mcn.issuer.adapter.logging;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

class PanMaskerTest {

  @ParameterizedTest
  @CsvSource(
      delimiter = '|',
      value = {
        "card 9704360000004417 approved|card 970436******4417 approved",
        "pan=4111111111111111111|pan=411111*********1111",
        "rrn 626514000123 stan 000123|rrn 626514000123 stan 000123",
        "de90 020000012409210732440000097049900000000000|de90 020000012409210732440000097049900000000000"
      })
  void should_mask_pan_like_numbers_only__MCN_005_AC2(String input, String expected) {
    assertThat(PanMasker.mask(input)).isEqualTo(expected);
  }
}
```

Add `testImplementation("org.junit.jupiter:junit-jupiter-params")` to `build.gradle.kts`, re-run `./gradlew dependencies --write-locks`.
Run: `./gradlew test` → compilation failure (`PanMasker` missing).

**Step 2 — implement** `src/main/java/io/mcn/issuer/adapter/logging/PanMasker.java`

```java
package io.mcn.issuer.adapter.logging;

import java.util.regex.Matcher;
import java.util.regex.Pattern;

/** Keeps the first 6 and last 4 digits of standalone 13–19 digit runs (PCI DSS 3.4). */
public final class PanMasker {
  private static final Pattern PAN_LIKE = Pattern.compile("\\b\\d{13,19}\\b");

  private PanMasker() {}

  public static String mask(String text) {
    Matcher m = PAN_LIKE.matcher(text);
    StringBuilder out = new StringBuilder();
    while (m.find()) {
      String pan = m.group();
      m.appendReplacement(out, pan.substring(0, 6) + "*".repeat(pan.length() - 10) + pan.substring(pan.length() - 4));
    }
    m.appendTail(out);
    return out.toString();
  }
}
```

Run: `./gradlew test` → PASS. Commit: `feat(iss): PAN masker (MCN-005)`.

### Task 3: JSON logging with masking (AC2)

**Step 1 — failing test** `src/test/java/io/mcn/issuer/adapter/logging/LogbackMaskingTest.java`

```java
package io.mcn.issuer.adapter.logging;

import static java.nio.charset.StandardCharsets.UTF_8;
import static org.assertj.core.api.Assertions.assertThat;

import ch.qos.logback.classic.Level;
import ch.qos.logback.classic.LoggerContext;
import ch.qos.logback.classic.joran.JoranConfigurator;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.classic.spi.LoggingEvent;
import ch.qos.logback.core.OutputStreamAppender;
import org.junit.jupiter.api.Test;

class LogbackMaskingTest {

  @Test
  void should_write_masked_json_with_standard_fields__MCN_005_AC2() throws Exception {
    LoggerContext context = new LoggerContext();
    JoranConfigurator configurator = new JoranConfigurator();
    configurator.setContext(context);
    configurator.doConfigure(getClass().getResource("/logback.xml"));

    @SuppressWarnings("unchecked")
    var appender = (OutputStreamAppender<ILoggingEvent>) context.getLogger("ROOT").getAppender("JSON");
    var event = new LoggingEvent("test", context.getLogger("t"), Level.INFO, "authorized 9704360000004417", null, null);

    String json = new String(appender.getEncoder().encode(event), UTF_8);

    assertThat(json)
        .contains("\"ts\"")
        .contains("\"service\":\"issuer\"")
        .contains("authorized 970436******4417")
        .doesNotContain("9704360000004417");
  }
}
```

Run: `./gradlew test` → fails (no `logback.xml`).

**Step 2 — implement** `src/main/resources/logback.xml`

```xml
<configuration>
  <!-- docs/02 §7.6 fields; trace_id/span_id come from the OpenTelemetry Java agent's MDC injection. -->
  <appender name="JSON" class="ch.qos.logback.core.ConsoleAppender">
    <encoder class="net.logstash.logback.encoder.LogstashEncoder">
      <fieldNames>
        <timestamp>ts</timestamp>
        <version>[ignore]</version>
      </fieldNames>
      <customFields>{"service":"issuer"}</customFields>
      <jsonGeneratorDecorator class="net.logstash.logback.mask.MaskingJsonGeneratorDecorator">
        <valueMask>
          <value>\b(\d{6})\d{3,9}(\d{4})\b</value>
          <mask>$1******$2</mask>
        </valueMask>
      </jsonGeneratorDecorator>
    </encoder>
  </appender>
  <root level="INFO">
    <appender-ref ref="JSON"/>
  </root>
</configuration>
```

Run: `./gradlew test` → PASS. (Check: if the resolved encoder version does not expand `$1/$2` in masks, the assertion shows it; then replace the decorator with a custom `JsonGeneratorDecorator` that calls `PanMasker.mask` and record a ruling.) Commit: `feat(iss): JSON logging with PAN masking (MCN-005)`.

### Task 4: Health and metrics QBeans (AC1, AC4)

**Step 1 — failing test** `src/test/java/io/mcn/issuer/adapter/http/HttpEndpointsTest.java`

```java
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
```

Run: `./gradlew test` → compilation failure.

**Step 2 — implement** (`src/main/java/io/mcn/issuer/adapter/http/`)

`Readiness.java`

```java
package io.mcn.issuer.adapter.http;

import java.util.concurrent.atomic.AtomicBoolean;

/** Readiness flag; turns false as soon as graceful shutdown begins. */
public final class Readiness {
  private final AtomicBoolean draining = new AtomicBoolean(false);

  public void startDraining() {
    draining.set(true);
  }

  public boolean isReady() {
    return !draining.get();
  }
}
```

`HealthServer.java` (framework code isolated from Q2 so it is testable without a deploy directory)

```java
package io.mcn.issuer.adapter.http;

import io.javalin.Javalin;

/** Serves /health/live and /health/ready. */
public final class HealthServer {
  private static final String UP = "{\"status\":\"UP\"}";
  private static final String DRAINING = "{\"status\":\"DRAINING\"}";
  private final Readiness readiness;
  private Javalin app;

  public HealthServer(Readiness readiness) {
    this.readiness = readiness;
  }

  /** Starts on {@code port} (0 = random) and returns the bound port. */
  public int start(int port) {
    app =
        Javalin.create()
            .get("/health/live", ctx -> ctx.contentType("application/json").result(UP))
            .get(
                "/health/ready",
                ctx -> {
                  ctx.contentType("application/json");
                  if (readiness.isReady()) {
                    ctx.result(UP);
                  } else {
                    ctx.status(503).result(DRAINING);
                  }
                })
            .start(port);
    return app.port();
  }

  public void stop() {
    if (app != null) {
      app.stop();
    }
  }
}
```

`HttpEndpoints.java` (Q2 lifecycle)

```java
package io.mcn.issuer.adapter.http;

import org.jpos.q2.QBeanSupport;

/** Q2 bean exposing health endpoints; readiness drops before the server stops (NFR-09). */
public final class HttpEndpoints extends QBeanSupport {
  private final Readiness readiness = new Readiness();
  private final HealthServer server = new HealthServer(readiness);

  @Override
  protected void startService() {
    server.start(cfg.getInt("port", 8081));
  }

  @Override
  protected void stopService() throws InterruptedException {
    readiness.startDraining();
    Thread.sleep(cfg.getLong("drain-ms", 2000));
    server.stop();
  }
}
```

`MetricsEndpoint.java`

```java
package io.mcn.issuer.adapter.http;

import io.javalin.Javalin;
import io.micrometer.core.instrument.binder.jvm.JvmMemoryMetrics;
import io.micrometer.core.instrument.binder.jvm.JvmThreadMetrics;
import io.micrometer.prometheusmetrics.PrometheusConfig;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import org.jpos.q2.QBeanSupport;

/** Q2 bean serving Prometheus metrics on its own port (docs/02 §8: 9465). */
public final class MetricsEndpoint extends QBeanSupport {
  private final PrometheusMeterRegistry registry = new PrometheusMeterRegistry(PrometheusConfig.DEFAULT);
  private Javalin app;

  @Override
  protected void startService() {
    new JvmMemoryMetrics().bindTo(registry);
    new JvmThreadMetrics().bindTo(registry);
    app = Javalin.create().get("/metrics", ctx -> ctx.contentType("text/plain").result(registry.scrape())).start(cfg.getInt("port", 9465));
  }

  @Override
  protected void stopService() {
    if (app != null) {
      app.stop();
    }
  }
}
```

Check: Micrometer ≥ 1.13 uses the `io.micrometer.prometheusmetrics` package; older versions use `io.micrometer.prometheus`.

Deploy descriptors (`src/dist/deploy/`):

`00_logger.xml`
```xml
<logger name="Q2" class="org.jpos.q2.qbean.LoggerAdaptor">
  <log-listener class="org.jpos.util.SimpleLogListener"/>
</logger>
```

`40_http_endpoints.xml`
```xml
<http-endpoints class="io.mcn.issuer.adapter.http.HttpEndpoints" logger="Q2">
  <property name="port" value="${env:HTTP_PORT:8081}"/>
  <property name="drain-ms" value="${env:DRAIN_MS:2000}"/>
</http-endpoints>
```

`45_metrics_endpoint.xml`
```xml
<metrics-endpoint class="io.mcn.issuer.adapter.http.MetricsEndpoint" logger="Q2">
  <property name="port" value="${env:METRICS_PORT:9465}"/>
</metrics-endpoint>
```

Check: jPOS expands environment references in descriptor properties, but the exact syntax (`${env:NAME:default}`, `$env{NAME}` or `${NAME:default}`) depends on the jPOS version. Confirm it in the resolved version's documentation and adjust all three descriptors in one commit.

Run: `./gradlew test` → PASS; `./gradlew run` then `curl -fsS localhost:8081/health/ready` and `curl -fsS localhost:9465/metrics | grep jvm_memory` in another shell; Ctrl-C shows the beans stopping. Commit: `feat(iss): health and metrics endpoints as Q2 beans (MCN-005)`.

### Task 5: Architecture guard

`src/test/java/io/mcn/issuer/ArchitectureTest.java`

```java
package io.mcn.issuer;

import com.tngtech.archunit.junit.AnalyzeClasses;
import com.tngtech.archunit.junit.ArchTest;
import com.tngtech.archunit.lang.ArchRule;
import static com.tngtech.archunit.library.Architectures.layeredArchitecture;

@AnalyzeClasses(packages = "io.mcn.issuer")
class ArchitectureTest {
  @ArchTest
  static final ArchRule hexagonal =
      layeredArchitecture()
          .consideringOnlyDependenciesInLayers()
          .withOptionalLayers(true)
          .layer("Domain").definedBy("..domain..")
          .layer("Application").definedBy("..application..")
          .layer("Adapter").definedBy("..adapter..")
          .whereLayer("Adapter").mayNotBeAccessedByAnyLayer()
          .whereLayer("Application").mayOnlyBeAccessedByLayers("Adapter")
          .whereLayer("Domain").mayOnlyBeAccessedByLayers("Application", "Adapter");
}
```

Run: `./gradlew test` → PASS (domain/application empty, allowed by `withOptionalLayers`). Commit: `test(iss): hexagonal architecture rule (MCN-005)`.

### Task 6: Container with OpenTelemetry agent (AC3)

`Dockerfile`

```dockerfile
FROM eclipse-temurin:21-jdk AS build
WORKDIR /src
COPY . .
RUN ./gradlew --no-daemon installDist

FROM eclipse-temurin:21-jre
WORKDIR /app
ADD https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/latest/download/opentelemetry-javaagent.jar /otel/opentelemetry-javaagent.jar
COPY --from=build /src/build/install/issuer-jpos/ /app/
EXPOSE 8000 8081 9465
ENV JAVA_TOOL_OPTIONS="-javaagent:/otel/opentelemetry-javaagent.jar"
ENTRYPOINT ["/app/bin/issuer-jpos"]
```

Check: the start script runs Q2 from `/app`; `deploy/` must sit next to `bin/` (it does because `distributions` copies `src/dist`). If Q2 does not find `deploy/`, set `WORKDIR /app` (already) and verify with `docker compose ... logs issuer`.

`compose.yaml`

```yaml
services:
  issuer:
    build: { context: ./issuer-jpos }
    environment:
      OTEL_SERVICE_NAME: issuer
      OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT}
      OTEL_EXPORTER_OTLP_PROTOCOL: grpc
      OTEL_METRICS_EXPORTER: none
      OTEL_LOGS_EXPORTER: none
    ports: ["8081:8081", "9465:9465"]
    stop_grace_period: 35s
    depends_on: [otel-collector, postgres]
```

### Task 7: End-to-end verification

Run the same sequence as MCN-005-GW Task 9 against ports 8081 (health) and 9465 (metrics), service `issuer`, and look for `"service":"issuer"` JSON lines containing `trace_id` for the traced request. Then superpowers:verification-before-completion → superpowers:requesting-code-review → superpowers:finishing-a-development-branch. PR title: `feat(iss): service skeleton with health, metrics, logs, traces (MCN-005)`.

## AC → verification

| AC | Proof |
| --- | --- |
| MCN-005-AC1 | `HttpEndpointsTest`; curls on 8081 and 9465 |
| MCN-005-AC2 | `PanMaskerTest`, `LogbackMaskingTest` |
| MCN-005-AC3 | Tempo lookup of the traced request (Java agent) |
| MCN-005-AC4 | draining test; `docker compose stop issuer` exits 0 within the grace period |
