-- +goose Up
CREATE TABLE key_rotation (
    id           BIGSERIAL PRIMARY KEY,
    key_type     TEXT NOT NULL CHECK (key_type IN ('ZPK','ZAK')),
    status       TEXT NOT NULL CHECK (status IN ('RUNNING','COMPLETED','FAILED')),
    steps        JSONB NOT NULL,
    new_kcv      CHAR(6),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
    id          BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    event_type  TEXT NOT NULL,
    detail      JSONB NOT NULL
);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE key_rotation;
