package io.mcn.issuer.adapter.logging;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

class PanMaskerTest {

  @ParameterizedTest
  @CsvSource(
      delimiter = '|',
      value = {
        "card 9704360000004417 approved|card 970436******4417 approved",
        "pan=4111111111111111111|pan=411111*********1111",
        "rrn 626514000123 stan 000123|rrn 626514000123 stan 000123",
        "de90 020000012409210732440000097049900000000000|de90 020000012409210732440000097049900000000000"
      })
  void should_mask_pan_like_numbers_only__MCN_005_AC2(String input, String expected) {
    assertThat(PanMasker.mask(input)).isEqualTo(expected);
  }
}
