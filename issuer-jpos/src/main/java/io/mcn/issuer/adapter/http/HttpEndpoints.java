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
