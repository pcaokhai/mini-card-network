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

  @Test
  @org.junit.jupiter.api.DisplayName(
      "SEC-G15: a resent 0800/161 carrying the key that is already ACTIVE answers 00 and changes"
          + " nothing, so the retired key stays the one before it")
  void should_acknowledgeWithoutANewRow_when_theKeyIsAlreadyActive() throws Exception {
    SecurityModule securityModule = mock(SecurityModule.class);
    KeyStoreRepository keyStoreRepository = mock(KeyStoreRepository.class);
    AuditLogRepository auditLogRepository = mock(AuditLogRepository.class);
    byte[] zmk = HexFormat.of().parseHex("00".repeat(16));
    byte[] cryptogramUnderZmk = HexFormat.of().parseHex("22".repeat(16));
    byte[] activeUnderLmk = HexFormat.of().parseHex("ee".repeat(16));
    when(securityModule.unwrapUnderKey(eq(cryptogramUnderZmk), eq(zmk)))
        .thenReturn(HexFormat.of().parseHex("11".repeat(16)));
    when(securityModule.unwrap(activeUnderLmk))
        .thenReturn(HexFormat.of().parseHex("11".repeat(16)));
    when(keyStoreRepository.findActive("ZAK", "970499"))
        .thenReturn(
            java.util.Optional.of(
                new KeyStoreRow(
                    7,
                    "ZAK",
                    "970499",
                    HexFormat.of().formatHex(activeUnderLmk),
                    "DDEEFF",
                    "ACTIVE",
                    java.time.Instant.now(),
                    null,
                    null)));
    ReceiveKeyChange receiveKeyChange =
        new ReceiveKeyChange(securityModule, keyStoreRepository, auditLogRepository, zmk, "970499");

    ISOMsg request = new ISOMsg("0800");
    request.set(70, "161");
    request.set(48, "ZAK:" + HexFormat.of().formatHex(cryptogramUnderZmk));

    assertThat(receiveKeyChange.receive(request)).isTrue();
    verify(keyStoreRepository, org.mockito.Mockito.never()).insert(any());
    verify(keyStoreRepository, org.mockito.Mockito.never())
        .activate(org.mockito.ArgumentMatchers.anyLong());
    org.mockito.Mockito.verifyNoInteractions(auditLogRepository);
  }

  @Test
  @org.junit.jupiter.api.DisplayName(
      "SEC-G18: a key change carrying a key that was already RETIRED is a replay - declined, no"
          + " insert, no activation, and audited")
  void should_rejectAReplayedRetiredKey() throws Exception {
    SecurityModule securityModule = mock(SecurityModule.class);
    KeyStoreRepository keyStoreRepository = mock(KeyStoreRepository.class);
    AuditLogRepository auditLogRepository = mock(AuditLogRepository.class);
    byte[] zmk = HexFormat.of().parseHex("00".repeat(16));
    byte[] cryptogramUnderZmk = HexFormat.of().parseHex("22".repeat(16));
    byte[] activeUnderLmk = HexFormat.of().parseHex("aa".repeat(16));
    byte[] retiredUnderLmk = HexFormat.of().parseHex("bb".repeat(16));
    when(securityModule.unwrapUnderKey(eq(cryptogramUnderZmk), eq(zmk)))
        .thenReturn(HexFormat.of().parseHex("11".repeat(16))); // the old key, replayed
    when(securityModule.unwrap(activeUnderLmk))
        .thenReturn(HexFormat.of().parseHex("99".repeat(16)));
    when(securityModule.unwrap(retiredUnderLmk))
        .thenReturn(HexFormat.of().parseHex("11".repeat(16)));
    when(keyStoreRepository.findActive("ZAK", "970499"))
        .thenReturn(java.util.Optional.of(row(8, activeUnderLmk, "ACTIVE")));
    when(keyStoreRepository.findRetired("ZAK", "970499"))
        .thenReturn(java.util.List.of(row(7, retiredUnderLmk, "RETIRED")));
    ReceiveKeyChange receiveKeyChange =
        new ReceiveKeyChange(securityModule, keyStoreRepository, auditLogRepository, zmk, "970499");

    ISOMsg request = new ISOMsg("0800");
    request.set(70, "161");
    request.set(48, "ZAK:" + HexFormat.of().formatHex(cryptogramUnderZmk));

    assertThat(receiveKeyChange.receive(request)).isFalse(); // the caller answers 0810 RC 96
    verify(keyStoreRepository, org.mockito.Mockito.never()).insert(any());
    verify(keyStoreRepository, org.mockito.Mockito.never())
        .activate(org.mockito.ArgumentMatchers.anyLong());
    verify(auditLogRepository)
        .record(
            eq("issuer"),
            eq("key_change.replay_rejected"),
            eq("key_store"),
            eq("7"),
            any(),
            org.mockito.ArgumentMatchers.argThat(
                (String after) -> after.contains("ZAK") && !after.contains("1111")));
  }

  private static KeyStoreRow row(long id, byte[] underLmk, String status) {
    return new KeyStoreRow(
        id,
        "ZAK",
        "970499",
        HexFormat.of().formatHex(underLmk),
        "DDEEFF",
        status,
        java.time.Instant.now(),
        "RETIRED".equals(status) ? java.time.Instant.now() : null,
        null);
  }
}
