package io.mcn.issuer.adapter.iso;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.AcquirerLinkRepository;
import org.jpos.iso.ISOMsg;
import org.junit.jupiter.api.Test;

class HandleNetworkManagementKeyChangeTest {

  @Test
  void should_respond_00_when_receive_key_change_succeeds__MCN_504_AC1() throws Exception {
    AcquirerLinkRepository links = mock(AcquirerLinkRepository.class);
    ReceiveKeyChange receiveKeyChange = mock(ReceiveKeyChange.class);
    when(receiveKeyChange.receive(org.mockito.ArgumentMatchers.any())).thenReturn(true);

    ISOMsg request = new ISOMsg("0800");
    request.set(70, "161");

    ISOMsg response = new HandleNetworkManagement(links, receiveKeyChange).handle(request);

    assertThat(response.getString(39)).isEqualTo("00");
  }

  @Test
  void should_respond_96_when_receive_key_change_fails__MCN_504_AC1() throws Exception {
    AcquirerLinkRepository links = mock(AcquirerLinkRepository.class);
    ReceiveKeyChange receiveKeyChange = mock(ReceiveKeyChange.class);
    when(receiveKeyChange.receive(org.mockito.ArgumentMatchers.any())).thenReturn(false);

    ISOMsg request = new ISOMsg("0800");
    request.set(70, "161");

    ISOMsg response = new HandleNetworkManagement(links, receiveKeyChange).handle(request);

    assertThat(response.getString(39)).isEqualTo("96");
  }

  @Test
  void should_respond_96_when_key_rotation_not_configured__MCN_504_AC1() throws Exception {
    AcquirerLinkRepository links = mock(AcquirerLinkRepository.class);

    ISOMsg request = new ISOMsg("0800");
    request.set(70, "161");

    ISOMsg response = new HandleNetworkManagement(links).handle(request);

    assertThat(response.getString(39)).isEqualTo("96");
  }
}
