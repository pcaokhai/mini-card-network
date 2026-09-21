package io.mcn.issuer.adapter.iso;

import static org.assertj.core.api.Assertions.assertThat;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.io.File;
import java.nio.file.Files;
import java.util.Iterator;
import java.util.Map;
import java.util.TreeMap;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.packager.GenericPackager;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.MethodSource;

class PackagerVectorTest {

  private static GenericPackager packager() throws Exception {
    return new GenericPackager("src/dist/cfg/iso87ascii.xml");
  }

  static java.util.stream.Stream<File> vectors() throws Exception {
    return Files.list(new File("../contracts/iso8583/vectors").toPath())
        .map(java.nio.file.Path::toFile)
        .filter(f -> f.getName().endsWith(".json"));
  }

  @ParameterizedTest
  @MethodSource("vectors")
  void should_pack_and_unpack_byte_identical__MCN_102_AC2(File file) throws Exception {
    ObjectMapper mapper = new ObjectMapper();
    JsonNode vector = mapper.readTree(file);
    String mti = vector.get("mti").asText();
    String expectedPacked = vector.get("packed").asText();

    ISOMsg msg = new ISOMsg();
    msg.setPackager(packager());
    msg.setMTI(mti);
    Iterator<Map.Entry<String, JsonNode>> it = vector.get("fields").fields();
    while (it.hasNext()) {
      Map.Entry<String, JsonNode> e = it.next();
      msg.set(Integer.parseInt(e.getKey()), e.getValue().asText());
    }
    String packed = new String(msg.pack());
    assertThat(packed).as("pack " + file.getName()).isEqualTo(expectedPacked);

    ISOMsg unpacked = new ISOMsg();
    unpacked.setPackager(packager());
    unpacked.unpack(expectedPacked.getBytes());
    assertThat(unpacked.getMTI()).isEqualTo(mti);
    var expectedFields = new TreeMap<Integer, String>();
    vector
        .get("fields")
        .fields()
        .forEachRemaining(e -> expectedFields.put(Integer.parseInt(e.getKey()), e.getValue().asText()));
    for (var entry : expectedFields.entrySet()) {
      assertThat(unpacked.getString(entry.getKey()))
          .as("DE " + entry.getKey())
          .isEqualTo(entry.getValue());
    }
  }
}
