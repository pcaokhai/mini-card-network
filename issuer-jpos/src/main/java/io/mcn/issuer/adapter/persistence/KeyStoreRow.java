package io.mcn.issuer.adapter.persistence;

import java.time.Instant;

public record KeyStoreRow(
    long id,
    String keyType,
    String counterparty,
    String keyUnderLmkHex,
    String kcv,
    String status,
    Instant activatedAt,
    Instant retiredAt,
    Instant createdAt) {}
