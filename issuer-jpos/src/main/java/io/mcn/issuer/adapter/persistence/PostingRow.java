package io.mcn.issuer.adapter.persistence;

/** {@code account} is a customer account number or a GL code, whichever the posting targets. */
public record PostingRow(String account, String direction, long amount, String currency) {}
