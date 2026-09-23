package io.mcn.issuer.adapter.txn;

import com.zaxxer.hikari.HikariDataSource;
import io.mcn.issuer.adapter.crypto.JCESecurityModule;
import io.mcn.issuer.adapter.crypto.PvvCalculator;
import io.mcn.issuer.adapter.persistence.CardRepository;
import io.mcn.issuer.adapter.persistence.KeyStoreRepository;
import io.mcn.issuer.application.SecurityModule;
import java.io.Serializable;
import java.util.Arrays;
import java.util.HexFormat;
import java.util.Optional;
import org.jpos.core.Configurable;
import org.jpos.core.Configuration;
import org.jpos.core.ConfigurationException;
import org.jpos.iso.ISOMsg;
import org.jpos.transaction.Context;
import org.jpos.transaction.TransactionParticipant;
import org.jpos.util.Destroyable;

/**
 * Verifies the Retail MAC (DE 64) under the issuer's ZAK, then the PIN block's PVV (DE 52) under
 * the issuer's ZPK, per docs/03 §11 and MCN-503.md. A bad MAC sets RC 96 and aborts before any PIN
 * check runs (docs/03 §11: "A MAC failure is answered with RC 96"), so pin_try_count only ever
 * moves on a MAC-verified (genuine) request. A bad PVV sets RC 55 and increments
 * card.pin_try_count; the third consecutive bad PVV also sets card.status = PIN_BLOCKED and RC 75.
 * DE 52 is unset from the request unconditionally right after this participant runs, so no later
 * participant, log line or persisted row ever sees it (PCI DSS rule 2, root CLAUDE.md §6.2).
 */
public class VerifySecurity implements TransactionParticipant, Configurable, Destroyable {

  private static final int MAX_PIN_TRIES = 3;

  // docs/03 §3 - matches the counterparty id ReceiveKeyChange writes key_store rows under.
  private static final String LAB_ACQUIRER_ID = "970499";

  private SecurityModule securityModule;
  private CardRepository cardRepository;
  private KeyStoreRepository keyStoreRepository;
  private byte[] zak;
  private byte[] zpk;
  private HikariDataSource dataSource;

  /** No-arg constructor for Q2's {@code QFactory.newInstance}; see {@link #setConfiguration}. */
  public VerifySecurity() {}

  public VerifySecurity(
      SecurityModule securityModule, CardRepository cardRepository, byte[] zak, byte[] zpk) {
    this(securityModule, cardRepository, null, zak, zpk);
  }

  /** {@code keyStoreRepository} may be {@code null} to skip the dual-key retry (MCN-504-AC2). */
  public VerifySecurity(
      SecurityModule securityModule,
      CardRepository cardRepository,
      KeyStoreRepository keyStoreRepository,
      byte[] zak,
      byte[] zpk) {
    this.securityModule = securityModule;
    this.cardRepository = cardRepository;
    this.keyStoreRepository = keyStoreRepository;
    this.zak = zak;
    this.zpk = zpk;
  }

  @Override
  public void setConfiguration(Configuration cfg) throws ConfigurationException {
    this.dataSource = TxnDataSource.fromConfig(cfg);
    this.cardRepository = new CardRepository(dataSource);
    this.keyStoreRepository = new KeyStoreRepository(dataSource);
    this.securityModule = new JCESecurityModule(env("LMK_TEST_VALUE_HEX"));
    this.zak = HexFormat.of().parseHex(env("ZAK_HEX"));
    this.zpk = HexFormat.of().parseHex(env("ZPK_HEX"));
  }

  @Override
  public void destroy() {
    if (dataSource != null) {
      dataSource.close();
    }
  }

  private static String env(String name) {
    String value = System.getenv(name);
    return value != null ? value : System.getProperty(name);
  }

  @Override
  public int prepare(long id, Serializable context) {
    Context ctx = (Context) context;
    ISOMsg request = ctx.get(TxnContextKeys.REQUEST);

    try {
      if (!verifyMac(request)) {
        ctx.put(TxnContextKeys.RESPONSE_CODE, "96");
        return ABORTED;
      }

      if (!request.hasField(52)) {
        return PREPARED;
      }

      long cardId = ctx.get(TxnContextKeys.CARD_ID);
      String pan = ctx.get(TxnContextKeys.PAN);
      return verifyPin(ctx, request, cardId, pan);
    } catch (Exception e) {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "96");
      return ABORTED;
    } finally {
      if (request != null && request.hasField(52)) {
        request.unset(52);
      }
    }
  }

  private boolean verifyMac(ISOMsg request) throws Exception {
    byte[] receivedMac = request.getBytes(64);
    request.unset(64);
    byte[] packed = request.pack();
    if (Arrays.equals(receivedMac, securityModule.computeMac(packed, zak))) {
      return true;
    }
    return recentlyRetiredKey("ZAK")
        .map(
            retiredZak -> Arrays.equals(receivedMac, securityModule.computeMac(packed, retiredZak)))
        .orElse(false);
  }

  /**
   * Dual-key acceptance (MCN-504-AC2, docs/03 §9): on a mismatch against the active key, retry once
   * against the most recently retired key of the same type, within the shared {@link
   * KeyStoreRepository#DUAL_KEY_WINDOW}.
   */
  private Optional<byte[]> recentlyRetiredKey(String keyType) {
    if (keyStoreRepository == null) {
      return Optional.empty();
    }
    return keyStoreRepository
        .findRecentlyRetired(keyType, LAB_ACQUIRER_ID, KeyStoreRepository.DUAL_KEY_WINDOW)
        .map(row -> securityModule.unwrap(HexFormat.of().parseHex(row.keyUnderLmkHex())));
  }

  private int verifyPin(Context ctx, ISOMsg request, long cardId, String pan) throws Exception {
    byte[] pinBlockUnderZpk = request.getBytes(52);
    String storedPvv = cardRepository.findPvv(cardId).orElse(null);

    if (pvvMatches(pinBlockUnderZpk, pan, zpk, storedPvv)) {
      cardRepository.resetPinTryCount(cardId);
      return PREPARED;
    }

    int newCount = cardRepository.incrementPinTryCount(cardId);
    if (newCount >= MAX_PIN_TRIES) {
      cardRepository.blockForPin(cardId);
      ctx.put(TxnContextKeys.RESPONSE_CODE, "75");
    } else {
      ctx.put(TxnContextKeys.RESPONSE_CODE, "55");
    }
    return ABORTED;
  }

  private boolean pvvMatches(byte[] pinBlockUnderZpk, String pan, byte[] key, String storedPvv)
      throws Exception {
    if (pvvMatchesUnder(pinBlockUnderZpk, pan, key, storedPvv)) {
      return true;
    }
    Optional<byte[]> retiredZpk = recentlyRetiredKey("ZPK");
    return retiredZpk.isPresent()
        && pvvMatchesUnder(pinBlockUnderZpk, pan, retiredZpk.get(), storedPvv);
  }

  private boolean pvvMatchesUnder(byte[] pinBlockUnderZpk, String pan, byte[] key, String storedPvv)
      throws Exception {
    byte[] clearPinBlock = securityModule.decryptPinBlock(pinBlockUnderZpk, key);
    String pin = extractPin(clearPinBlock, pan);
    return PvvCalculator.computePvv(pan, pin).equals(storedPvv);
  }

  /**
   * Reverses ISO 9564-1 format 0: XOR the clear block with {@code 0000} + the 12 rightmost PAN
   * digits excluding the check digit, then read the {@code '0'} + length nibble + PIN + {@code 'F'}
   * padding - identical to web-next's {@code buildPinBlock} (pinblock.ts), inverted.
   */
  private static String extractPin(byte[] clearPinBlock, String pan) {
    String blockHex = HexFormat.of().formatHex(clearPinBlock);
    String panDigits = pan.substring(0, pan.length() - 1);
    panDigits = panDigits.substring(panDigits.length() - 12);
    String panField = "0000" + panDigits;

    StringBuilder pinField = new StringBuilder(16);
    for (int i = 0; i < 16; i++) {
      int a = Character.digit(blockHex.charAt(i), 16);
      int b = Character.digit(panField.charAt(i), 16);
      pinField.append(Integer.toHexString(a ^ b));
    }
    int pinLength = Character.digit(pinField.charAt(1), 16);
    return pinField.substring(2, 2 + pinLength);
  }
}
