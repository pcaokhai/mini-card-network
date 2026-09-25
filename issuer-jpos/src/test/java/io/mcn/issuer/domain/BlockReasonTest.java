package io.mcn.issuer.domain;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.Test;

class BlockReasonTest {
  @Test
  void should_keepLostAndStolen_when_mappingToACardStatus() {
    assertThat(BlockReason.LOST.cardStatus()).isEqualTo("LOST");
    assertThat(BlockReason.STOLEN.cardStatus()).isEqualTo("STOLEN");
  }

  @Test
  void should_block_when_fraudSuspectedOrCustomerRequest() {
    assertThat(BlockReason.FRAUD_SUSPECTED.cardStatus()).isEqualTo("BLOCKED");
    assertThat(BlockReason.CUSTOMER_REQUEST.cardStatus()).isEqualTo("BLOCKED");
  }

  @Test
  void should_beEmpty_when_reasonIsNotInTheContractEnum() {
    assertThat(BlockReason.parse("BORED")).isEmpty();
    assertThat(BlockReason.parse(null)).isEmpty();
    assertThat(BlockReason.parse("LOST")).contains(BlockReason.LOST);
  }
}
