package io.mcn.issuer.adapter.persistence;

public record Card(
    long id, long accountId, String bin, String panLast4, String expiryYymm, String status) {}
