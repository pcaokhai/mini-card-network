package io.mcn.issuer.adapter.iso;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;

import io.mcn.issuer.adapter.persistence.AcquirerLinkRepository;
import org.jpos.iso.ISOMsg;
import org.junit.jupiter.api.Test;

/**
 * NetworkManagementListener is the first listener on the ISO server: whatever it claims never
 * reaches AuthorizationListener or ReversalListener behind it.
 */
class HandleNetworkManagementRoutingTest {

  private final HandleNetworkManagement handler =
      new HandleNetworkManagement(mock(AcquirerLinkRepository.class), mock(ReceiveKeyChange.class));

  @Test
  void leavesReversalAdvicesToTheReversalListener__MCN_401() throws Exception {
    assertThat(handler.handle(new ISOMsg("0420"))).isNull();
    assertThat(handler.handle(new ISOMsg("0421"))).isNull();
  }

  @Test
  void leavesAuthorizationAndFinancialRequestsToTheAuthorizationListener() throws Exception {
    assertThat(handler.handle(new ISOMsg("0100"))).isNull();
    assertThat(handler.handle(new ISOMsg("0200"))).isNull();
  }

  @Test
  void answersAnMtiNoListenerOwnsWithAFormatError() throws Exception {
    ISOMsg response = handler.handle(new ISOMsg("0600"));

    assertThat(response.getString(39)).isEqualTo("30");
  }
}
