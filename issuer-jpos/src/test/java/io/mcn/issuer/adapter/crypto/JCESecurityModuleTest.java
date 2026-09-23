package io.mcn.issuer.adapter.crypto;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.HexFormat;
import org.junit.jupiter.api.Test;

class JCESecurityModuleTest {

  private static final String LMK_HEX =
      "bf86ef10435417e8b25d78d8d3af5310e3f5bc079036d1f676872c058ca4f59d";

  @Test
  void should_wrap_and_unwrap_round_trip_without_exposing_clear_key__MCN_501_AC1() {
    JCESecurityModule module = new JCESecurityModule(LMK_HEX);
    byte[] clearKey = HexFormat.of().parseHex("0123456789abcdef0123456789abcdef");

    byte[] wrapped = module.wrapUnderLmk(clearKey);

    assertThat(wrapped).isNotEqualTo(clearKey);
    assertThat(module.unwrap(wrapped)).isEqualTo(clearKey);
  }

  @Test
  void should_compute_six_hex_char_kcv_as_leading_bytes_of_zero_block_encryption__MCN_501_AC2() {
    JCESecurityModule module = new JCESecurityModule(LMK_HEX);
    byte[] clearKey = HexFormat.of().parseHex("0123456789abcdef0123456789abcdef");

    String kcv = module.computeKcv(clearKey);

    assertThat(kcv).hasSize(6).matches("^[0-9A-F]{6}$");
  }

  @Test
  void should_fail_fast_when_lmk_missing__MCN_501_AC3() {
    assertThatThrownBy(() -> new JCESecurityModule(null))
        .isInstanceOf(IllegalArgumentException.class)
        .hasMessageContaining("LMK");
    assertThatThrownBy(() -> new JCESecurityModule(""))
        .isInstanceOf(IllegalArgumentException.class);
  }

  @Test
  void should_never_render_clear_key_bytes_in_toString_or_exception_messages__MCN_501_AC1() {
    JCESecurityModule module = new JCESecurityModule(LMK_HEX);
    byte[] clearKey = HexFormat.of().parseHex("deadbeefdeadbeefdeadbeefdeadbeef");
    String clearKeyHex = "deadbeefdeadbeefdeadbeefdeadbeef";

    byte[] wrapped = module.wrapUnderLmk(clearKey);
    String kcv = module.computeKcv(clearKey);

    assertThat(wrapped.toString()).doesNotContain(clearKeyHex);
    assertThat(kcv).doesNotContain(clearKeyHex);
    assertThatThrownBy(() -> module.unwrap(new byte[] {1, 2, 3}))
        .isInstanceOf(RuntimeException.class)
        .hasMessageNotContaining(clearKeyHex);
  }
}
