package io.mcn.issuer.domain;

import java.util.Arrays;
import java.util.Optional;

/** What a financial request (docs/03 §6, DE 3) does to the customer's account. */
public enum TransactionType {
  PURCHASE("000000", CustomerEffect.DEBIT),
  CASH("010000", CustomerEffect.DEBIT),
  REFUND("200000", CustomerEffect.CREDIT),
  BALANCE_INQUIRY("310000", CustomerEffect.NONE);

  /** Direction money moves on the customer's account when the request is approved. */
  public enum CustomerEffect {
    DEBIT,
    CREDIT,
    NONE
  }

  private final String processingCode;
  private final CustomerEffect customerEffect;

  TransactionType(String processingCode, CustomerEffect customerEffect) {
    this.processingCode = processingCode;
    this.customerEffect = customerEffect;
  }

  public CustomerEffect customerEffect() {
    return customerEffect;
  }

  public static Optional<TransactionType> fromProcessingCode(String processingCode) {
    return Arrays.stream(values()).filter(t -> t.processingCode.equals(processingCode)).findFirst();
  }
}
