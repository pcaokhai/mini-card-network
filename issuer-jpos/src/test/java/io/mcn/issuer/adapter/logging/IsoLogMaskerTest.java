package io.mcn.issuer.adapter.logging;

import static org.assertj.core.api.Assertions.assertThat;

import org.jpos.iso.ISOMsg;
import org.junit.jupiter.api.Test;

class IsoLogMaskerTest {

  @Test
  void should_mask_pan_key_material_pin_block_and_icc_data__MCN_102_AC3() throws Exception {
    ISOMsg msg = new ISOMsg();
    msg.setMTI("0200");
    msg.set(2, "9704360000004417");
    msg.set(48, "KT=ZPK;KC=DEADBEEF;KCV=ABCDEF");
    msg.set(52, "7A3F09C21B84D6E0");
    msg.set(55, "9F2608A1B2C3D4E5F607189F2701809F3602001C");
    msg.set(11, "000123"); // not sensitive, must survive untouched

    String dump = IsoLogMasker.maskedDump(msg);

    assertThat(dump).contains("970436******4417").doesNotContain("9704360000004417");
    assertThat(dump).contains("48: [MASKED key-material]").doesNotContain("DEADBEEF");
    assertThat(dump).contains("52: [MASKED pin-block]").doesNotContain("7A3F09C21B84D6E0");
    assertThat(dump)
        .contains("55: [MASKED emv]")
        .doesNotContain("9F2608A1B2C3D4E5F607189F2701809F3602001C");
    assertThat(dump).contains("000123");
  }
}
