package io.mcn.issuer.adapter.http;

import io.javalin.config.RoutesConfig;
import io.javalin.http.Context;
import io.mcn.issuer.adapter.persistence.KeyStoreRow;
import java.time.Duration;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.function.Supplier;

/**
 * `GET /v1/keys/issuer` (MCN-501-AC2): {@code contracts/openapi.yaml}'s {@code KeyInfo[]} shape -
 * type/counterparty/kcv/status/daysRemaining, never {@code key_under_lmk}.
 */
public final class KeysController {
  // ponytail: fixed lifetime until a rotation story (MCN-504) makes it configurable per key type.
  private static final int LIFETIME_DAYS = 365;

  private final Supplier<List<KeyStoreRow>> keys;

  public KeysController(Supplier<List<KeyStoreRow>> keys) {
    this.keys = keys;
  }

  public void registerRoutes(RoutesConfig routes) {
    routes.get("/v1/keys/issuer", this::listKeys);
  }

  private void listKeys(Context ctx) {
    ctx.json(keys.get().stream().map(this::keyInfo).toList());
  }

  private Map<String, Object> keyInfo(KeyStoreRow row) {
    Map<String, Object> m = new LinkedHashMap<>();
    m.put("keyType", row.keyType());
    m.put("counterparty", row.counterparty());
    m.put("kcv", row.kcv());
    m.put("status", row.status());
    m.put("activatedAt", row.activatedAt() == null ? null : row.activatedAt().toString());
    m.put("daysRemaining", daysRemaining(row.activatedAt()));
    m.put("lifetimeDays", LIFETIME_DAYS);
    return m;
  }

  private static int daysRemaining(Instant activatedAt) {
    if (activatedAt == null) {
      return LIFETIME_DAYS;
    }
    long daysSince = Duration.between(activatedAt, Instant.now()).toDays();
    return (int) Math.max(0, LIFETIME_DAYS - daysSince);
  }
}
