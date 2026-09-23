package io.mcn.issuer.adapter.persistence;

import java.time.LocalDate;

/**
 * MCN-402: the subset of a located original transaction's {@code tran_log} columns {@code
 * LocateAndReverse} needs, including the row's own id (never exposed by {@link TranLogRow}, which
 * is built for dedupe-replay use, not for a caller that has to reference the row back).
 */
public record OriginalTransactionRow(
    long id, LocalDate businessDate, long cardId, long amount, String currency, String status) {}
