package io.mcn.issuer.adapter.persistence;

import java.time.LocalDate;

/** The subset of {@code tran_log} columns MCN-302a/302b write; later stories extend inserts. */
public record TranLogRow(
    LocalDate businessDate,
    String mti,
    String tranType,
    String processingCode,
    String acquirerId,
    String tid,
    String mid,
    String stan,
    String transmissionDtRaw,
    String rrn,
    long amount,
    String currency,
    Long cardId,
    String status,
    String responseCode,
    String authCode,
    String declineReason,
    Long balance) {

  /** A row with no answered balance: everything but a balance inquiry (R-2). */
  public TranLogRow(
      LocalDate businessDate,
      String mti,
      String tranType,
      String processingCode,
      String acquirerId,
      String tid,
      String mid,
      String stan,
      String transmissionDtRaw,
      String rrn,
      long amount,
      String currency,
      Long cardId,
      String status,
      String responseCode,
      String authCode,
      String declineReason) {
    this(
        businessDate,
        mti,
        tranType,
        processingCode,
        acquirerId,
        tid,
        mid,
        stan,
        transmissionDtRaw,
        rrn,
        amount,
        currency,
        cardId,
        status,
        responseCode,
        authCode,
        declineReason,
        null);
  }
}
