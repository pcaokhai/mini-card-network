package io.mcn.issuer.adapter.persistence;

public record AccountSummary(
    String accountNo, String currency, long ledgerBalance, long availableBalance) {}
