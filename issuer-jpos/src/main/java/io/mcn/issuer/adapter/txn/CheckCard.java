package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.crypto.CardCrypto;
import io.mcn.issuer.adapter.persistence.Card;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.domain.CardLifecycle;
import java.io.Serializable;
import java.time.LocalDate;
import java.util.Optional;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;
import org.jpos.util.Destroyable;

/**
 * Looks up the card by PAN hash and declines RC 14 (unknown), or by the card's status as {@link
 * CardLifecycle} reads it for the business date: RC 54 expired, RC 75 PIN_BLOCKED, RC 62 BLOCKED,
 * LOST or STOLEN (docs/03 §8). The Admin API reads status with the same rule, so both agree.
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
    // Same rule and date source the Admin API reads status with (CARDS-G18).
    LocalDate businessDate = ctx.get(TxnContextKeys.BUSINESS_DATE);
    String status =
        CardLifecycle.effectiveStatus(
            card.status(),
            card.expiryYymm(),
            businessDate == null ? LocalDate.now() : businessDate);
    Optional<String> declineRc = declineCodeFor(status);
    if (declineRc.isPresent()) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, declineRc.get());
      return ABORTED;
    }

    ctx.put(TxnContextKeys.ACCOUNT_ID, card.accountId());
    ctx.put(TxnContextKeys.CARD_ID, card.id());
    return PREPARED;
  }

  /** docs/03 §8: 54 expired, 75 PIN tries exceeded, 62 restricted (v1 uses 62 for the rest). */
  private static Optional<String> declineCodeFor(String effectiveStatus) {
    return switch (effectiveStatus) {
      case "ACTIVE" -> Optional.empty();
      case "EXPIRED" -> Optional.of("54");
      case "PIN_BLOCKED" -> Optional.of("75");
      default -> Optional.of("62");
    };
  }
}
