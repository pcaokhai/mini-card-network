package io.mcn.issuer.adapter.iso;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import io.mcn.issuer.adapter.persistence.AuditLogRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRow;
import io.mcn.issuer.application.SecurityModule;
import java.util.HexFormat;
import org.jpos.iso.ISOMsg;
import org.junit.jupiter.api.Test;

class ReceiveKeyChangeTest {

  @Test
  void should_unwrap_store_pending_then_activate_and_audit__MCN_504_AC1_AC3() throws Exception {
    SecurityModule securityModule = mock(SecurityModule.class);
    KeyStoreRepository keyStoreRepository = mock(KeyStoreRepository.class);
    AuditLogRepository auditLogRepository = mock(AuditLogRepository.class);
    byte[] zmk = HexFormat.of().parseHex("00".repeat(16));
    byte[] newClearKey = HexFormat.of().parseHex("11".repeat(16));
    byte[] cryptogramUnderZmk = HexFormat.of().parseHex("22".repeat(16));

    when(securityModule.unwrapUnderKey(eq(cryptogramUnderZmk), eq(zmk))).thenReturn(newClearKey);
    when(securityModule.wrapUnderLmk(newClearKey))
        .thenReturn(HexFormat.of().parseHex("ff".repeat(16)));
    when(securityModule.computeKcv(newClearKey)).thenReturn("DDEEFF");
    when(keyStoreRepository.insert(any())).thenReturn(42L);

    ReceiveKeyChange receiveKeyChange =
        new ReceiveKeyChange(securityModule, keyStoreRepository, auditLogRepository, zmk, "970499");

    ISOMsg request = new ISOMsg("0800");
    request.set(70, "161");
    request.set(48, "ZAK:" + HexFormat.of().formatHex(cryptogramUnderZmk));
    request.set(53, "01");

    boolean accepted = receiveKeyChange.receive(request);

    assertThat(accepted).isTrue();
    verify(keyStoreRepository)
        .insert(
            argThat(
                row ->
                    row instanceof KeyStoreRow r
                        && "ZAK".equals(r.keyType())
                        && "970499".equals(r.counterparty())
                        && "DDEEFF".equals(r.kcv())
                        && "PENDING".equals(r.status())));
    verify(keyStoreRepository).activate(42L);
    verify(auditLogRepository)
        .record(eq("issuer"), eq("key_change.activated"), eq("key_store"), eq("42"), any(), any());
    assertThat(request.hasField(48)).isFalse();
  }

  @Test
  void should_reject_when_key_type_prefix_is_unrecognized__MCN_504_AC1() throws Exception {
    SecurityModule securityModule = mock(SecurityModule.class);
    KeyStoreRepository keyStoreRepository = mock(KeyStoreRepository.class);
    AuditLogRepository auditLogRepository = mock(AuditLogRepository.class);
    byte[] zmk = HexFormat.of().parseHex("00".repeat(16));

    ReceiveKeyChange receiveKeyChange =
        new ReceiveKeyChange(securityModule, keyStoreRepository, auditLogRepository, zmk, "970499");

    ISOMsg request = new ISOMsg("0800");
    request.set(70, "161");
    request.set(48, "not-a-valid-field");

    boolean accepted = receiveKeyChange.receive(request);

    assertThat(accepted).isFalse();
    assertThat(request.hasField(48)).isFalse();
  }

  private static <T> T argThat(org.mockito.ArgumentMatcher<T> matcher) {
    return org.mockito.ArgumentMatchers.argThat(matcher);
  }
}
