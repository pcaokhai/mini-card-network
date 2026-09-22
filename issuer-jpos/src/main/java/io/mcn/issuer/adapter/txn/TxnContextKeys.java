package io.mcn.issuer.adapter.txn;

/** Shared {@code org.jpos.transaction.Context} keys read/written across the participant chain. */
public final class TxnContextKeys {
  private TxnContextKeys() {}

  public static final String REQUEST = "REQUEST";
  public static final String SOURCE = "SOURCE";
  public static final String RESPONSE_CODE = "RESPONSE_CODE";
  public static final String DECLINE_REASON = "DECLINE_REASON";
  public static final String AMOUNT = "AMOUNT";
  public static final String PROCESSING_CODE = "PROCESSING_CODE";
  public static final String ACQUIRER_ID = "ACQUIRER_ID";
  public static final String RRN = "RRN";
  public static final String IS_DUPLICATE = "IS_DUPLICATE";
  public static final String STORED_RESPONSE = "STORED_RESPONSE";
  public static final String PAN = "PAN";
  public static final String PAN_HASH = "PAN_HASH";
  public static final String BUSINESS_DATE = "BUSINESS_DATE";
  public static final String ACCOUNT_ID = "ACCOUNT_ID";
  public static final String CARD_ID = "CARD_ID";
}
