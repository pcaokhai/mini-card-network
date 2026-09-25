package io.mcn.issuer.domain;

import java.util.Arrays;
import java.util.Optional;

/** Why an operator blocks a card (contracts/openapi.yaml blockCard), and the status it leaves. */
public enum BlockReason {
  CUSTOMER_REQUEST("BLOCKED"),
  LOST("LOST"),
  STOLEN("STOLEN"),
  FRAUD_SUSPECTED("BLOCKED");

  private final String cardStatus;

  BlockReason(String cardStatus) {
    this.cardStatus = cardStatus;
  }

  public String cardStatus() {
    return cardStatus;
  }

  public static Optional<BlockReason> parse(String value) {
    return Arrays.stream(values()).filter(r -> r.name().equals(value)).findFirst();
  }
}
