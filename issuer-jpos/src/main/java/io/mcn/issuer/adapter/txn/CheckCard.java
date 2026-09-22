package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.crypto.CardCrypto;
import io.mcn.issuer.adapter.persistence.Card;
import io.mcn.issuer.adapter.persistence.CardRepository;
import java.io.Serializable;
import java.time.LocalDate;
import java.time.YearMonth;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;
import org.jpos.util.Destroyable;

/**
 * Looks up the card by PAN hash and declines RC 14 (unknown), RC 62 (blocked/lost/stolen) or RC 54
 * (expired card, DE 14 YYMM semantics: expired once the business date's YYMM exceeds it).
 */
public class CheckCard implements TransactionParticipant, Configurable, Destroyable {

  private CardRepository cardRepository;
  private CardCrypto cardCrypto;
  private HikariDataSource dataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public CheckCard() {}

  public CheckCard(CardRepository cardRepository) {
    this.cardRepository = cardRepository;
  }

  public CheckCard(CardRepository cardRepository, CardCrypto cardCrypto) {
    this.cardRepository = cardRepository;
    this.cardCrypto = cardCrypto;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.dataSource = TxnDataSource.fromConfig(cfg);
    this.cardRepository = new CardRepository(dataSource);
    this.cardCrypto = new CardCrypto(env("PAN_ENCRYPTION_KEY_HEX"), env("PAN_HMAC_KEY_HEX"));
  }

  @Override
  public void destroy() {
    if (dataSource != null) {
      dataSource.close();
    }
  }

  private static String env(String name) {
    String value = System.getenv(name);
    return value != null ? value : System.getProperty(name);
  }

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    byte[] panHash = ctx.get(TxnContextKeys.PAN_HASH);
    if (panHash == null) {
      String pan = ctx.get(TxnContextKeys.PAN);
      panHash = cardCrypto.hash(pan);
    }

    var found = cardRepository.findByPanHash(panHash);
    if (found.isEmpty()) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "14");
      return ABORTED;
    }

    Card card = found.get();
    if (isNonActive(card.status())) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "62");
      return ABORTED;
    }

    LocalDate businessDate = ctx.get(TxnContextKeys.BUSINESS_DATE);
    if (businessDate != null && isExpired(card.expiryYymm(), businessDate)) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "54");
      return ABORTED;
    }

    ctx.put(TxnContextKeys.ACCOUNT_ID, card.accountId());
    ctx.put(TxnContextKeys.CARD_ID, card.id());
    return PREPARED;
  }

  private static boolean isNonActive(String status) {
    return "BLOCKED".equals(status) || "LOST".equals(status) || "STOLEN".equals(status);
  }

  private static boolean isExpired(String expiryYymm, LocalDate businessDate) {
    int year = 2000 + Integer.parseInt(expiryYymm.substring(0, 2));
    int month = Integer.parseInt(expiryYymm.substring(2, 4));
    YearMonth expiry = YearMonth.of(year, month);
    return YearMonth.from(businessDate).isAfter(expiry);
  }
}
