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
    // Javalin 7 moved route registration off the Javalin instance and onto
    // config.routes (io.javalin.config.RoutesConfig implements JavalinDefaultRoutingApi);
    // there is no fluent .get(...) directly on the object Javalin.create() returns.
    app =
        Javalin.create(
                config ->
                    config
                        .routes
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
                            }))
            .start(port);
    return app.port();
  }

  public void stop() {
    if (app != null) {
      app.stop();
    }
  }
}
