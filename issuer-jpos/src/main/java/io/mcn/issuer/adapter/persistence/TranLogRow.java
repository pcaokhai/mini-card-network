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
    String declineReason) {}
