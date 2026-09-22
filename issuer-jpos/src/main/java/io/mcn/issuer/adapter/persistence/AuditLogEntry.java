package io.mcn.issuer.adapter.persistence;

import java.time.Instant;

public record AuditLogEntry(
    long id,
    String actor,
    String action,
    String entityType,
    String entityId,
    String beforeState,
    String afterState,
    Instant createdAt) {}
