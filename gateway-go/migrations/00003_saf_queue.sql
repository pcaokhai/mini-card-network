-- +goose Up
-- ponytail: docs/assets/baseline-schema.sql's saf_queue FKs (tran_id, tran_business_date) into a
-- partitioned tran_log with a composite PK. This repo's tran_log (migrations/00002_transactions.sql)
-- is not partitioned and has a single-column PK (id) - tran_state_history already made the same
-- simplification (tran_id BIGINT REFERENCES tran_log(id)), so saf_queue follows the established
-- local pattern instead of the baseline's partitioned-schema shape.
CREATE TABLE saf_queue (
    id                 BIGSERIAL PRIMARY KEY,
    tran_id            BIGINT NOT NULL REFERENCES tran_log(id),
    mti                CHAR(4) NOT NULL CHECK (mti IN ('0120','0220','0420')),
    payload_enc        BYTEA NOT NULL,
    status             TEXT NOT NULL DEFAULT 'PENDING'
                       CHECK (status IN ('PENDING','IN_FLIGHT','ACKED','DEAD')),
    attempts           INTEGER NOT NULL DEFAULT 0,
    max_attempts       INTEGER NOT NULL DEFAULT 20,
    next_retry_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error         TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    acked_at           TIMESTAMPTZ
);

CREATE INDEX ix_saf_due ON saf_queue(next_retry_at) WHERE status IN ('PENDING', 'IN_FLIGHT');

ALTER TABLE tran_log ADD COLUMN late_response_code CHAR(2);
ALTER TABLE tran_log ADD COLUMN late_response_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE tran_log DROP COLUMN late_response_at;
ALTER TABLE tran_log DROP COLUMN late_response_code;
DROP TABLE saf_queue;
