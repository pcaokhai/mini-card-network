package io.mcn.issuer.adapter.persistence;

/** A row-locked snapshot of {@code account}, read via {@link AccountLockRepository#lockAndGet}. */
public record AccountRow(
    long id, long availableBalance, long ledgerBalance, long overdraftLimit, long version) {}
