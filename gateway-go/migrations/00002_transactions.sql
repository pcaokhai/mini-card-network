-- +goose Up
CREATE TABLE merchant (
  mid        VARCHAR(15) PRIMARY KEY,
  name       TEXT NOT NULL,
  mcc        CHAR(4) NOT NULL,
  city       TEXT,
  country    CHAR(2) NOT NULL DEFAULT 'VN',
  status     TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUSPENDED','CLOSED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE terminal (
  tid          CHAR(8) PRIMARY KEY,
  mid          VARCHAR(15) NOT NULL REFERENCES merchant(mid),
  status       TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','DISABLED')),
  last_tstan   INTEGER NOT NULL DEFAULT 0 CHECK (last_tstan BETWEEN 0 AND 999999),
  batch_no     INTEGER NOT NULL DEFAULT 1,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tran_log (
  id                     BIGSERIAL PRIMARY KEY,
  business_date          DATE NOT NULL,
  client_request_id      TEXT NOT NULL,
  tran_type              TEXT NOT NULL,
  mti                    CHAR(4),
  processing_code        CHAR(6),
  tid                    CHAR(8) NOT NULL REFERENCES terminal(tid),
  mid                    VARCHAR(15) NOT NULL,
  network_stan           CHAR(6),
  rrn                    CHAR(12) NOT NULL,
  masked_pan             VARCHAR(19),
  amount                 BIGINT NOT NULL,
  currency               CHAR(3) NOT NULL,
  pos_entry_mode         CHAR(3),
  state                  TEXT NOT NULL CHECK (state IN
                          ('CREATED','SENT','APPROVED','DECLINED','TIMED_OUT',
                           'REVERSAL_PENDING','REVERSED','FAILED')),
  response_code          CHAR(2),
  auth_code              CHAR(6),
  sent_at                TIMESTAMPTZ,
  responded_at           TIMESTAMPTZ,
  latency_ms             INTEGER,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_acq_rrn   ON tran_log(rrn);
CREATE INDEX ix_acq_state ON tran_log(state) WHERE state IN ('SENT','TIMED_OUT','REVERSAL_PENDING');

CREATE TABLE tran_state_history (
  id                 BIGSERIAL PRIMARY KEY,
  tran_id            BIGINT NOT NULL REFERENCES tran_log(id),
  from_state         TEXT,
  to_state           TEXT NOT NULL,
  reason             TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE idempotency_record (
    key            TEXT NOT NULL,
    route          TEXT NOT NULL,
    request_hash   TEXT NOT NULL,
    status         INTEGER NOT NULL,
    body           JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (key, route)
);

INSERT INTO merchant (mid, name, mcc) VALUES ('GOCPHO000000001', 'Ca phe Goc Pho', '5814');
INSERT INTO terminal (tid, mid) VALUES ('00000042', 'GOCPHO000000001');

-- +goose Down
DROP TABLE idempotency_record;
DROP TABLE tran_state_history;
DROP TABLE tran_log;
DROP TABLE terminal;
DROP TABLE merchant;
