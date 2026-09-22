package io.mcn.issuer.adapter.txn;

import io.mcn.issuer.adapter.persistence.Card;
import io.mcn.issuer.adapter.persistence.CardRepository;
import java.io.Serializable;
import java.time.LocalDate;
import java.time.YearMonth;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;

/**
 * Looks up the card by PAN hash and declines RC 14 (unknown), RC 62 (blocked/lost/stolen) or RC
 * 54 (expired card, DE 14 YYMM semantics: expired once the business date's YYMM exceeds it).
 */
public class CheckCard implements TransactionParticipant {

  private final CardRepository cardRepository;

  public CheckCard(CardRepository cardRepository) {
    this.cardRepository = cardRepository;
  }

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    byte[] panHash = ctx.get(TxnContextKeys.PAN_HASH);

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
