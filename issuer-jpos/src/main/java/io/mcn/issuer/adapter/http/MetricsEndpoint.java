package io.mcn.issuer.adapter.http;

import io.javalin.Javalin;
import io.micrometer.core.instrument.binder.jvm.JvmMemoryMetrics;
import io.micrometer.core.instrument.binder.jvm.JvmThreadMetrics;
import io.micrometer.prometheusmetrics.PrometheusConfig;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import org.jpos.q2.QBeanSupport;

/** Q2 bean serving Prometheus metrics on its own port (docs/02 §8: 9465). */
public final class MetricsEndpoint extends QBeanSupport {
  private final PrometheusMeterRegistry registry =
      new PrometheusMeterRegistry(PrometheusConfig.DEFAULT);
  private Javalin app;

  @Override
  protected void startService() {
    new JvmMemoryMetrics().bindTo(registry);
    new JvmThreadMetrics().bindTo(registry);
    app =
        Javalin.create(
                config ->
                    config.routes.get(
                        "/metrics", ctx -> ctx.contentType("text/plain").result(registry.scrape())))
            .start(cfg.getInt("port", 9465));
  }

  @Override
  protected void stopService() {
    if (app != null) {
      app.stop();
    }
  }
}
