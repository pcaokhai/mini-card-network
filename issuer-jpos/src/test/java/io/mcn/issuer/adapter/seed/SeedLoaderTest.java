package io.mcn.issuer.adapter.seed;

import static org.assertj.core.api.Assertions.assertThat;

import io.mcn.issuer.adapter.crypto.CardCrypto;
import io.mcn.issuer.adapter.persistence.Card;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.TestDataSources;
import java.nio.file.Path;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class SeedLoaderTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void loadsAllFourFixtureCardsAndIsIdempotent() throws Exception {
    var ds = TestDataSources.migrated(postgres);
    var crypto =
        new CardCrypto(
            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
            "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210");
    var fixturePath = Path.of("../contracts/fixtures/cards.json");

    new SeedLoader().load(ds, crypto, fixturePath);
    new SeedLoader().load(ds, crypto, fixturePath); // idempotent: running twice must not fail

    var cards = new CardRepository(ds);
    byte[] normalPanHash = crypto.hash("9704360000004417");
    var found = cards.findByPanHash(normalPanHash);
    assertThat(found).isPresent();
    assertThat(found.get().status()).isEqualTo("ACTIVE");

    byte[] blockedPanHash = crypto.hash("9704360000003310");
    assertThat(cards.findByPanHash(blockedPanHash)).map(Card::status).contains("BLOCKED");
  }
}
