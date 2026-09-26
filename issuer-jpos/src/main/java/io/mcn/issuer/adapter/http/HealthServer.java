package io.mcn.issuer.adapter.http;

import io.javalin.Javalin;

/**
 * Serves /health/live and /health/ready, plus the {@code /v1/cards*} Admin API (MCN-308) when a
 * {@link CardAdminController} is supplied - docs/04 §1 puts both on the same issuer:8081 base URL,
 * and Javalin 7 only allows route registration inside one {@code Javalin.create(config -> ...)}.
 */
public final class HealthServer {
  private static final String UP = "{\"status\":\"UP\"}";
  private static final String DRAINING = "{\"status\":\"DRAINING\"}";
  private final Readiness readiness;
  private final CardAdminController cardAdmin;
  private final KeysController keys;
  private Javalin app;

  public HealthServer(Readiness readiness) {
    this(readiness, null);
  }

  public HealthServer(Readiness readiness, CardAdminController cardAdmin) {
    this(readiness, cardAdmin, null);
  }

  public HealthServer(Readiness readiness, CardAdminController cardAdmin, KeysController keys) {
    this.readiness = readiness;
    this.cardAdmin = cardAdmin;
    this.keys = keys;
  }

  /** Starts on {@code port} (0 = random) on every interface and returns the bound port. */
  public int start(int port) {
    return start(null, port);
  }

  /**
   * Starts on {@code host} only ({@code null} = every interface). Tests bind 127.0.0.1, the address
   * they call: a random port on the wildcard address can share its number with another local server
   * bound to 127.0.0.1 (macOS allows it), which then answers the test's requests.
   */
  public int start(String host, int port) {
    // Javalin 7 moved route registration off the Javalin instance and onto
    // config.routes (io.javalin.config.RoutesConfig implements JavalinDefaultRoutingApi);
    // there is no fluent .get(...) directly on the object Javalin.create() returns.
    app =
        Javalin.create(
                config -> {
                  config
                      .routes
                      .before(ApiProblems::assignTraceId)
                      .exception(Exception.class, ApiProblems::internal)
                      .error(404, ApiProblems::unmatchedRoute)
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
                          });
                  if (cardAdmin != null) {
                    cardAdmin.registerRoutes(config.routes);
                  }
                  if (keys != null) {
                    keys.registerRoutes(config.routes);
                  }
                })
            .start(host, port);
    return app.port();
  }

  public void stop() {
    if (app != null) {
      app.stop();
    }
  }
}
