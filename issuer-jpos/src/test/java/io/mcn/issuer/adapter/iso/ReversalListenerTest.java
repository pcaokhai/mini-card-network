package io.mcn.issuer.adapter.iso;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.AcquirerLinkRepository;
import java.util.Optional;
import org.jpos.iso.ISOMsg;
import org.jpos.iso.ISOSource;
import org.junit.jupiter.api.Test;

class ReversalListenerTest {

  @Test
  void process_returnsFalseForNonClass04_MCN_402_AC1() throws Exception {
    var listener = new ReversalListener(mock(AcquirerLinkRepository.class), "reversal-txn-mgr");

    assertThat(listener.process(mock(ISOSource.class), new ISOMsg("0200"))).isFalse();
  }

  @Test
  void process_notSignedOnRespondsRC91DirectlyWithoutQueueing_MCN_402_AC1() throws Exception {
    var links = mock(AcquirerLinkRepository.class);
    when(links.findStatus(anyString())).thenReturn(Optional.empty());
    var listener = new ReversalListener(links, "reversal-txn-mgr");
    var source = mock(ISOSource.class);

    boolean handled = listener.process(source, build0420());

    assertThat(handled).isTrue();
    var captor = org.mockito.ArgumentCaptor.forClass(ISOMsg.class);
    org.mockito.Mockito.verify(source).send(captor.capture());
    assertThat(captor.getValue().getString(39)).isEqualTo("91");
  }

  @Test
  void process_class04SignedOnAcceptsForQueueing_MCN_402_AC1() throws Exception {
    var links = mock(AcquirerLinkRepository.class);
    when(links.findStatus(anyString())).thenReturn(Optional.of("SIGNED_ON"));
    var listener = new ReversalListener(links, "reversal-txn-mgr");

    assertThat(listener.process(mock(ISOSource.class), build0420())).isTrue();
  }

  private static ISOMsg build0420() throws Exception {
    ISOMsg msg = new ISOMsg("0420");
    msg.set(11, "000123");
    msg.set(90, "0200" + "000123" + "0922140000" + "970499     " + "00000000000");
    return msg;
  }
}
