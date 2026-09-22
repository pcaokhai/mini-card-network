package io.mcn.issuer.adapter.persistence;

/** A single {@code card_limit} row: either an amount ceiling, a count ceiling, or both. */
public record CardLimit(String tranType, String period, Long maxAmount, Integer maxCount) {}
