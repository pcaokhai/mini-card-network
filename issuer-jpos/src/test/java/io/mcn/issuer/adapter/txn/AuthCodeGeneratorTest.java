package io.mcn.issuer.adapter.txn;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.Test;

class AuthCodeGeneratorTest {
  @Test
  void generatesSixAlphanumericCharacters() {
    String code = new AuthCodeGenerator().generate();
    assertThat(code).hasSize(6).matches("[A-Z0-9]{6}");
  }

  @Test
  void ratelyProducesDistinctCodes() {
    var gen = new AuthCodeGenerator();
    assertThat(gen.generate()).isNotEqualTo(gen.generate());
  }
}
