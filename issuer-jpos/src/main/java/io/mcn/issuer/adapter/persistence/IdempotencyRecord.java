package io.mcn.issuer.adapter.persistence;

public record IdempotencyRecord(String key, String route, String requestHash, int status, String body) {}
