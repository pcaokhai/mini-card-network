package io.mcn.issuer.adapter.persistence;

/** A single {@code velocity_counter} row: today's (or this period's) running count and amount. */
public record VelocityCounterRow(
    long cardId, String tranType, String period, String periodKey, int txnCount, long txnAmount) {}
