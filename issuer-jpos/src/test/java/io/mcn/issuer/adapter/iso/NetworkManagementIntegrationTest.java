package io.mcn.issuer.adapter.iso;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertThrows;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.persistence.AcquirerLinkRepository;
import io.mcn.issuer.adapter.persistence.JdbcAcquirerLinkRepository;
import java.net.ServerSocket;
import java.net.Socket;
import java.util.Map;
import javax.sql.DataSource;
import org.flywaydb.core.Flyway;
import org.jpos.core.SimpleConfiguration;
import org.jpos.iso.ISOException;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.ISOServer;
import org.jpos.iso.channel.NACChannel;
import org.jpos.iso.packager.GenericPackager;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;

/**
 * Drives the real jPOS ISOServer/NACChannel stack over a raw socket, proving the wire format and
 * jPOS's own framing behavior (not a hand-rolled server or a hand-rolled encoder).
 */
@Testcontainers
class NetworkManagementIntegrationTest {

  @Container
  static PostgreSQLContainer<?> postgres = new PostgreSQLContainer<>("postgres:16-alpine");

  private ISOServer server;
  private AcquirerLinkRepository links;
  private int port;

  private static int freePort() throws Exception {
    try (ServerSocket s = new ServerSocket(0)) {
      return s.getLocalPort();
    }
  }

  private static GenericPackager packager() throws ISOException {
    return new GenericPackager("src/dist/cfg/iso87ascii.xml");
  }

  @BeforeEach
  void start() throws Exception {
    HikariConfig cfg = new HikariConfig();
    cfg.setJdbcUrl(postgres.getJdbcUrl());
    cfg.setUsername(postgres.getUsername());
    cfg.setPassword(postgres.getPassword());
    DataSource dataSource = new HikariDataSource(cfg);
    Flyway.configure().dataSource(dataSource).load().migrate();
    links = new JdbcAcquirerLinkRepository(dataSource);

    port = freePort();
    NACChannel channel = new NACChannel(packager(), new byte[0]);
    server = new ISOServer(port, channel, 10);
    server.setConfiguration(new SimpleConfiguration());
    server.addISORequestListener(new NetworkManagementListener(links));
    new Thread(server, "test-iso-server").start();
    Thread.sleep(200); // let the accept loop bind before the test connects
  }

  @AfterEach
  void stop() {
    server.shutdown();
  }

  private byte[] send(byte[] body) throws Exception {
    try (Socket socket = new Socket("127.0.0.1", port)) {
      socket.setSoTimeout(2000);
      var out = new java.io.DataOutputStream(socket.getOutputStream());
      out.writeShort(body.length);
      out.write(body);
      var in = new java.io.DataInputStream(socket.getInputStream());
      int len = in.readUnsignedShort();
      byte[] resp = new byte[len];
      in.readFully(resp);
      return resp;
    }
  }

  private ISOMsg buildIso(String mti, Map<Integer, String> fields) throws ISOException {
    ISOMsg msg = new ISOMsg();
    msg.setPackager(packager());
    msg.setMTI(mti);
    for (var entry : fields.entrySet()) {
      msg.set(entry.getKey(), entry.getValue());
    }
    return msg;
  }

  private ISOMsg unpack(byte[] bytes) throws ISOException {
    ISOMsg msg = new ISOMsg();
    msg.setPackager(packager());
    msg.unpack(bytes);
    return msg;
  }

  @Test
  void should_close_connection_on_unreadable_mti__MCN_201_AC4() {
    assertThrows(Exception.class, () -> send("XXXX8000000000000000".getBytes()));
  }

  @Test
  void should_answer_signon_with_0810_rc00_and_update_link_state__MCN_201_AC2() throws Exception {
    ISOMsg request = buildIso("0800", Map.of(7, "0921073300", 11, "000001", 70, "001"));
    byte[] resp = send(request.pack());
    ISOMsg response = unpack(resp);

    assertThat(response.getMTI()).isEqualTo("0810");
    assertThat(response.getString(39)).isEqualTo("00");
    assertThat(links.findStatus("970499")).contains("SIGNED_ON");
  }

  @Test
  void should_answer_echo_with_0810_rc00__MCN_201_AC2() throws Exception {
    ISOMsg request = buildIso("0800", Map.of(7, "0921073300", 11, "000002", 70, "301"));
    byte[] resp = send(request.pack());
    ISOMsg response = unpack(resp);

    assertThat(response.getMTI()).isEqualTo("0810");
    assertThat(response.getString(39)).isEqualTo("00");
  }

  @Test
  void should_reject_unsupported_network_management_code_with_rc30__MCN_201_AC2() throws Exception {
    ISOMsg request = buildIso("0800", Map.of(7, "0921073300", 11, "000005", 70, "999"));
    byte[] resp = send(request.pack());
    ISOMsg response = unpack(resp);

    assertThat(response.getString(39)).isEqualTo("30");
  }

  @Test
  void should_reject_financial_request_with_rc91__MCN_201_AC3() throws Exception {
    ISOMsg request =
        buildIso(
            "0200",
            Map.of(
                3, "000000",
                4, "000000100000",
                7, "0921073300",
                11, "000003",
                37, "626514000003"));
    byte[] resp = send(request.pack());
    ISOMsg response = unpack(resp);

    assertThat(response.getMTI()).isEqualTo("0210");
    assertThat(response.getString(39)).isEqualTo("91");
  }
}
