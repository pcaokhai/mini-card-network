package io.mcn.issuer.adapter.crypto;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.util.HexFormat;
import java.util.Map;
import org.junit.jupiter.api.Test;

class EmvTlvParserTest {

  @Test
  void should_parse_two_single_byte_length_tags__MCN_602_AC1() {
    // 9F36 (ATC) len 02 value 0001, 95 (TVR) len 05 value 0000000000
    byte[] tlv = HexFormat.of().parseHex("9F3602000195050000000000");

    Map<String, byte[]> tags = EmvTlvParser.parse(tlv);

    assertThat(tags).containsKey("9F36");
    assertThat(tags.get("9F36")).isEqualTo(HexFormat.of().parseHex("0001"));
    assertThat(tags).containsKey("95");
  }

  @Test
  void should_reject_truncated_length__MCN_602_AC1() {
    byte[] tlv = HexFormat.of().parseHex("9F3605"); // declares length 5, no value bytes follow

    assertThatThrownBy(() -> EmvTlvParser.parse(tlv))
        .isInstanceOf(EmvTlvParser.MalformedTlvException.class);
  }

  @Test
  void should_skip_unrecognized_but_well_formed_tags__MCN_602_AC1() {
    // DF01 (unrecognized) len 01 value FF, then 9C (type) len 01 value 00
    byte[] tlv = HexFormat.of().parseHex("DF0101FF9C0100");

    Map<String, byte[]> tags = EmvTlvParser.parse(tlv);

    assertThat(tags).containsKey("9C");
    assertThat(tags).containsKey("DF01"); // parsed structurally even though it has no named accessor
  }
}
