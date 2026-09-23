package io.mcn.issuer.adapter.persistence;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

@Testcontainers
class CardRepositoryTest {

  @Container
  static PostgreSQLContainer<?> postgres =
      new PostgreSQLContainer<>("postgres:16-alpine")
          .withDatabaseName("issuer")
          .withUsername("issuer")
          .withPassword("test");

  @Test
  void insertAndFindByPanHash() {
    var ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    var cards = new CardRepository(ds);

    long accountId = accounts.insert("ACC-0001", "704", 5_000_000L);
    byte[] panHash = "hash-bytes-not-real-crypto-here".getBytes();
    long cardId =
        cards.insert(accountId, "enc".getBytes(), panHash, "970436", "4417", "2811", "ACTIVE");

    var found = cards.findByPanHash(panHash);
    assertThat(found).isPresent();
    assertThat(found.get().id()).isEqualTo(cardId);
    assertThat(found.get().accountId()).isEqualTo(accountId);
  }

  @Test
  void should_update_emv_last_atc_only_when_strictly_increasing__MCN_602_AC2() {
    var ds = TestDataSources.migrated(postgres);
    var accounts = new AccountRepository(ds);
    var cards = new CardRepository(ds);

    long accountId = accounts.insert("ACC-0002", "704", 5_000_000L);
    byte[] panHash = "hash-bytes-not-real-crypto-atc".getBytes();
    long cardId =
        cards.insert(accountId, "enc".getBytes(), panHash, "970436", "4418", "2811", "ACTIVE");

    assertThat(cards.findEmvLastAtc(cardId)).isEmpty();

    boolean advanced = cards.updateEmvLastAtcIfIncreasing(cardId, 6);
    assertThat(advanced).isTrue();
    assertThat(cards.findEmvLastAtc(cardId)).contains(6);

    boolean rejected = cards.updateEmvLastAtcIfIncreasing(cardId, 6); // replay of the same ATC
    assertThat(rejected).isFalse();
    assertThat(cards.findEmvLastAtc(cardId)).contains(6);
  }
}
