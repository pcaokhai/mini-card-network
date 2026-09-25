package io.mcn.issuer.domain;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

class TransactionTypeTest {
  @Test
  @DisplayName("POS-G17: purchase and cash debit the customer, a refund credits them")
  void should_mapEachProcessingCodeToItsEffectOnTheCustomer() {
    assertThat(TransactionType.fromProcessingCode("000000")).contains(TransactionType.PURCHASE);
    assertThat(TransactionType.PURCHASE.customerEffect())
        .isEqualTo(TransactionType.CustomerEffect.DEBIT);
    assertThat(TransactionType.CASH.customerEffect())
        .isEqualTo(TransactionType.CustomerEffect.DEBIT);
    assertThat(TransactionType.fromProcessingCode("200000")).contains(TransactionType.REFUND);
    assertThat(TransactionType.REFUND.customerEffect())
        .isEqualTo(TransactionType.CustomerEffect.CREDIT);
  }

  @Test
  @DisplayName("POS-G17: a balance inquiry moves no money")
  void should_moveNoMoney_when_balanceInquiry() {
    assertThat(TransactionType.fromProcessingCode("310000"))
        .contains(TransactionType.BALANCE_INQUIRY);
    assertThat(TransactionType.BALANCE_INQUIRY.customerEffect())
        .isEqualTo(TransactionType.CustomerEffect.NONE);
  }

  @Test
  void should_beEmpty_when_processingCodeIsUnknown() {
    assertThat(TransactionType.fromProcessingCode("999999")).isEmpty();
    assertThat(TransactionType.fromProcessingCode(null)).isEmpty();
  }
}
