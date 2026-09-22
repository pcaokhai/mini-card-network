package io.mcn.issuer.adapter.persistence;

import java.time.Instant;
import java.util.List;

public record JournalEntryRow(
    long journalId, String entryType, Instant occurredAt, String rrn, List<PostingRow> postings) {}
