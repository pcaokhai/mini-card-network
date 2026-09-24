package io.mcn.issuer.adapter.seed;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.crypto.CardCrypto;
import java.nio.file.Path;
import org.flywaydb.core.Flyway;

/**
 * `make seed` entry point for the dev/test lab: never wired into the Q2 server deploy descriptor,
 * so it never runs against production. Env vars mirror NetworkManagementListener's.
 */
public final class SeedMain {
  private SeedMain() {}

  public static void main(String[] args) throws Exception {
    HikariConfig hikariConfig = new HikariConfig();
    hikariConfig.setJdbcUrl(System.getenv("DATABASE_URL"));
    hikariConfig.setUsername(System.getenv("DB_USER"));
    hikariConfig.setPassword(System.getenv("DB_PASSWORD"));
    HikariDataSource dataSource = new HikariDataSource(hikariConfig);
    Flyway.configure().dataSource(dataSource).load().migrate();

    CardCrypto crypto =
        new CardCrypto(System.getenv("PAN_ENCRYPTION_KEY_HEX"), System.getenv("PAN_HMAC_KEY_HEX"));
    // build.gradle.kts's `run` task sets workingDir = "src/dist" (for the real Q2 server's own
    // deploy-relative lookups), so from here the repo root is three levels up: src/dist ->
    // src -> issuer-jpos -> repo root.
    new SeedLoader().load(dataSource, crypto, Path.of("../../../contracts/fixtures/cards.json"));
  }
}
