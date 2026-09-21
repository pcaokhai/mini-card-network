-- +goose Up
CREATE TABLE link_state (
    id                    BIGSERIAL PRIMARY KEY,
    endpoint              TEXT NOT NULL UNIQUE,
    status                TEXT NOT NULL CHECK (status IN ('DISCONNECTED','CONNECTED','SIGNED_ON','DOWN')),
    last_echo_at          TIMESTAMPTZ,
    last_echo_latency_ms  INTEGER,
    p99_latency_ms        INTEGER,
    in_flight             INTEGER NOT NULL DEFAULT 0,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO link_state (endpoint, status) VALUES ('issuer', 'DISCONNECTED');

CREATE TABLE network_event (
    id              BIGSERIAL PRIMARY KEY,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    severity        TEXT NOT NULL CHECK (severity IN ('INFO','WARN','ERROR')),
    easy_text       TEXT NOT NULL,
    technical_text  TEXT NOT NULL
);
CREATE INDEX ix_network_event_occurred_at ON network_event (occurred_at DESC);

-- +goose Down
DROP TABLE network_event;
DROP TABLE link_state;
