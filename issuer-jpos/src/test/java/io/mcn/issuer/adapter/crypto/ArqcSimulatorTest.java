package io.mcn.issuer.adapter.crypto;

import static org.assertj.core.api.Assertions.assertThat;

import org.junit.jupiter.api.Test;

class ArqcSimulatorTest {

  @Test
  void should_verify_a_correctly_computed_arqc__MCN_602_AC2() {
    ArqcSimulator simulator = new ArqcSimulator();
    String pan = "9704360000000001";
    int atc = 1;
    byte[] un = new byte[] {0x01, 0x02, 0x03, 0x04};
    byte[] validArqc = simulator.computeArqcForTest(pan, atc, un);

    assertThat(simulator.verifyArqc(validArqc, pan, atc, un)).isTrue();
  }

  @Test
  void should_reject_a_tampered_arqc__MCN_602_AC2() {
    ArqcSimulator simulator = new ArqcSimulator();
    byte[] tampered = new byte[] {0, 0, 0, 0, 0, 0, 0, 0};

    assertThat(simulator.verifyArqc(tampered, "9704360000000001", 1, new byte[] {1, 2, 3, 4}))
        .isFalse();
  }

  @Test
  void should_compute_arpc_deterministically_from_arqc__MCN_602_AC3() {
    ArqcSimulator simulator = new ArqcSimulator();
    byte[] arqc = new byte[] {1, 2, 3, 4, 5, 6, 7, 8};

    byte[] arpc = simulator.computeArpc(arqc);

    assertThat(arpc).isNotEmpty();
    assertThat(simulator.computeArpc(arqc)).isEqualTo(arpc);
  }
}
