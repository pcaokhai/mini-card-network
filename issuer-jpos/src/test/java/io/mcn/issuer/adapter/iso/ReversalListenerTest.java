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

  private static final String LMK_HEX =
      "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff";
  private static final byte[] ZAK =
      java.util.HexFormat.of().parseHex("3132333435363738393a3b3c3d3e3f40");
  private static final io.mcn.issuer.adapter.crypto.JCESecurityModule HSM =
      new io.mcn.issuer.adapter.crypto.JCESecurityModule(LMK_HEX);
  private static final io.mcn.issuer.adapter.txn.ResponseMac MAC =
      new io.mcn.issuer.adapter.txn.ResponseMac(
          HSM,
          new io.mcn.issuer.application.SessionKeys() {
            @Override
            public byte[] active(String keyType) {
              return ZAK.clone();
            }

            @Override
            public Optional<byte[]> recentlyRetired(String keyType) {
              return Optional.empty();
            }
          });

  /** The 0430's own MAC: DE 128 (the echoed DE 90 sets the secondary bitmap), under the ZAK. */
  private static void assertMacedUnderTheZak(ISOMsg response) throws Exception {
    ISOMsg unsigned = (ISOMsg) response.clone();
    unsigned.unset(128);
    assertThat(response.getBytes(128)).isEqualTo(HSM.computeMac(unsigned.pack(), ZAK));
  }

  @Test
  void process_returnsFalseForNonClass04_MCN_402_AC1() throws Exception {
    var listener =
        new ReversalListener(mock(AcquirerLinkRepository.class), "reversal-txn-mgr", MAC);

    assertThat(listener.process(mock(ISOSource.class), new ISOMsg("0200"))).isFalse();
  }

  @Test
  void process_notSignedOnRespondsRC91DirectlyWithoutQueueing_MCN_402_AC1() throws Exception {
    var links = mock(AcquirerLinkRepository.class);
    when(links.findStatus(anyString())).thenReturn(Optional.empty());
    var listener = new ReversalListener(links, "reversal-txn-mgr", MAC);
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
    var listener = new ReversalListener(links, "reversal-txn-mgr", MAC);

    assertThat(listener.process(mock(ISOSource.class), build0420())).isTrue();
  }

  private static ISOMsg build0420() throws Exception {
    ISOMsg msg = new ISOMsg("0420");
    msg.setPackager(new org.jpos.iso.packager.GenericPackager("src/dist/cfg/iso87ascii.xml"));
    msg.set(39, "68");
    msg.set(128, "0123456789ABCDEF"); // the acquirer's own MAC over its 0420
    msg.set(11, "000123");
    msg.set(90, "0200" + "000123" + "0922140000" + "970499     " + "00000000000");
    return msg;
  }

  @Test
  @org.junit.jupiter.api.DisplayName(
      "NET-G19: the not-signed-on 0430 (RC 91) is MACed under the active ZAK")
  void should_macThe91Response_whenTheLinkIsNotSignedOn() throws Exception {
    var links = mock(AcquirerLinkRepository.class);
    when(links.findStatus(anyString())).thenReturn(Optional.empty());
    var source = mock(ISOSource.class);

    new ReversalListener(links, "reversal-txn-mgr", MAC).process(source, build0420());

    var captor = org.mockito.ArgumentCaptor.forClass(ISOMsg.class);
    org.mockito.Mockito.verify(source).send(captor.capture());
    assertThat(captor.getValue().getString(39)).isEqualTo("91");
    assertMacedUnderTheZak(captor.getValue());
  }

  @Test
  @org.junit.jupiter.api.DisplayName(
      "NET-G19: when the advice can't be queued, the 0430 says 96 (not recorded, repeat) and is"
          + " MACed")
  void should_answerAMacedNinetySix_whenTheAdviceCannotBeQueued() throws Exception {
    var links = mock(AcquirerLinkRepository.class);
    when(links.findStatus(anyString())).thenReturn(Optional.of("SIGNED_ON"));
    var source = mock(ISOSource.class);

    // no TransactionManager registered under this name: queueing fails
    new ReversalListener(links, "no-such-txn-mgr", MAC).process(source, build0420());

    var captor = org.mockito.ArgumentCaptor.forClass(ISOMsg.class);
    org.mockito.Mockito.verify(source).send(captor.capture());
    assertThat(captor.getValue().getMTI()).isEqualTo("0430");
    assertThat(captor.getValue().getString(39)).isEqualTo("96");
    assertMacedUnderTheZak(captor.getValue());
  }
}
