package io.mcn.issuer.adapter.seed;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.mcn.issuer.adapter.crypto.CardCrypto;
import java.nio.file.Files;
import java.nio.file.Path;
import java.sql.PreparedStatement;
import javax.sql.DataSource;

/** Loads {@code contracts/fixtures/cards.json} into account/card for the dev/test lab profile. */
public class SeedLoader {

  public void load(DataSource ds, CardCrypto crypto, Path fixturePath) throws Exception {
    JsonNode root = new ObjectMapper().readTree(Files.readString(fixturePath));
    for (JsonNode card : root.get("cards")) {
      seedOne(ds, crypto, card);
    }
  }

  private void seedOne(DataSource ds, CardCrypto crypto, JsonNode card) throws Exception {
    String pan = card.get("pan").asText();
    String accountNo = "ACC-" + card.get("cardRef").asText();
    byte[] panHash = crypto.hash(pan);

    try (var conn = ds.getConnection()) {
      try (var check = conn.prepareStatement("SELECT 1 FROM account WHERE account_no = ?")) {
        check.setString(1, accountNo);
        if (check.executeQuery().next()) return; // already seeded - idempotent
      }

      long ledgerBalance = card.get("balance").asLong();
      String currency = card.get("currency").asText();
      long accountId;
      try (PreparedStatement insertAccount =
          conn.prepareStatement(
              "INSERT INTO account (account_no, currency, ledger_balance, available_balance) "
                  + "VALUES (?, ?, ?, ?)",
              PreparedStatement.RETURN_GENERATED_KEYS)) {
        insertAccount.setString(1, accountNo);
        insertAccount.setString(2, currency);
        insertAccount.setLong(3, ledgerBalance);
        insertAccount.setLong(4, ledgerBalance);
        insertAccount.executeUpdate();
        var keys = insertAccount.getGeneratedKeys();
        keys.next();
        accountId = keys.getLong(1);
      }

      byte[] panEnc = crypto.encrypt(pan);
      String bin = pan.substring(0, 6);
      String panLast4 = pan.substring(pan.length() - 4);
      String expiryYymm = card.get("expiry").asText();
      String status = card.get("status").asText();
      String cardRef = card.get("cardRef").asText();
      String holderName = card.get("holderName").asText();
      try (PreparedStatement insertCard =
          conn.prepareStatement(
              "INSERT INTO card (account_id, pan_enc, pan_hash, bin, pan_last4, expiry_yymm,"
                  + " status, card_ref, holder_name) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)")) {
        insertCard.setLong(1, accountId);
        insertCard.setBytes(2, panEnc);
        insertCard.setBytes(3, panHash);
        insertCard.setString(4, bin);
        insertCard.setString(5, panLast4);
        insertCard.setString(6, expiryYymm);
        insertCard.setString(7, status);
        insertCard.setString(8, cardRef);
        insertCard.setString(9, holderName);
        insertCard.executeUpdate();
      }
    }
  }
}
