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
