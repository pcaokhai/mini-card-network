CREATE TABLE acquirer_link (
    id              BIGSERIAL PRIMARY KEY,
    acquirer_id     TEXT NOT NULL UNIQUE,
    status          TEXT NOT NULL CHECK (status IN ('DISCONNECTED','CONNECTED','SIGNED_ON','DOWN')),
    last_sign_on_at TIMESTAMPTZ,
    last_echo_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO acquirer_link (acquirer_id, status) VALUES ('970499', 'DISCONNECTED');
