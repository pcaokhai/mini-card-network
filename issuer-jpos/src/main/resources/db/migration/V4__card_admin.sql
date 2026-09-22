-- MCN-308: opaque card_ref for the Admin API (docs/05 §3 - never expose the internal BIGSERIAL
-- id), a holder_name for CardSummary/CardDetail, and the idempotency store for admin writes.
ALTER TABLE card ADD COLUMN card_ref TEXT NOT NULL DEFAULT ('crd_' || substr(md5(random()::text), 1, 12));
ALTER TABLE card ADD COLUMN holder_name TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX ux_card_ref ON card(card_ref);

CREATE TABLE idempotency_record (
    key            TEXT NOT NULL,
    route          TEXT NOT NULL,
    request_hash   TEXT NOT NULL,
    status         INTEGER NOT NULL,
    body           JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (key, route)
);
