-- Transcribed from docs/assets/baseline-schema.sql (issuer.* tables), schema-prefix stripped
-- since issuer-jpos connects to its own database per ADR-004. acquirer_link is NOT recreated
-- here: V1 (MCN-201) already owns it; V3 reconciles it with the baseline separately.

CREATE TABLE response_code (
  code        CHAR(2) PRIMARY KEY,
  description TEXT NOT NULL,
  category    TEXT NOT NULL CHECK (category IN ('APPROVED','DECLINED','ERROR','PICKUP'))
);

INSERT INTO response_code VALUES
  ('00','Approved','APPROVED'),
  ('05','Do not honor','DECLINED'),
  ('12','Invalid transaction','ERROR'),
  ('13','Invalid amount','ERROR'),
  ('14','Invalid card number','DECLINED'),
  ('30','Format error','ERROR'),
  ('41','Lost card','PICKUP'),
  ('43','Stolen card','PICKUP'),
  ('51','Insufficient funds','DECLINED'),
  ('54','Expired card','DECLINED'),
  ('55','Incorrect PIN','DECLINED'),
  ('57','Transaction not permitted to cardholder','DECLINED'),
  ('61','Exceeds withdrawal amount limit','DECLINED'),
  ('65','Exceeds withdrawal frequency limit','DECLINED'),
  ('75','PIN tries exceeded','DECLINED'),
  ('91','Issuer or switch inoperative','ERROR'),
  ('94','Duplicate transmission','ERROR'),
  ('96','System malfunction','ERROR');

CREATE TABLE system_state (
  id                    SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  current_business_date DATE NOT NULL,
  cutover_in_progress   BOOLEAN NOT NULL DEFAULT FALSE,
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE cutover_log (
  id                 BIGSERIAL PRIMARY KEY,
  from_business_date DATE NOT NULL,
  to_business_date   DATE NOT NULL,
  triggered_by       TEXT NOT NULL CHECK (triggered_by IN ('SCHEDULER','NETWORK_0800','MANUAL')),
  started_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at        TIMESTAMPTZ
);

CREATE TABLE account (
  id                BIGSERIAL PRIMARY KEY,
  account_no        VARCHAR(20) NOT NULL UNIQUE,
  currency          CHAR(3) NOT NULL,
  ledger_balance    BIGINT NOT NULL DEFAULT 0,
  available_balance BIGINT NOT NULL DEFAULT 0,
  overdraft_limit   BIGINT NOT NULL DEFAULT 0 CHECK (overdraft_limit >= 0),
  status            TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','FROZEN','CLOSED')),
  version           BIGINT NOT NULL DEFAULT 0,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT chk_available_floor CHECK (available_balance >= -overdraft_limit)
);

CREATE TABLE card (
  id             BIGSERIAL PRIMARY KEY,
  account_id     BIGINT NOT NULL REFERENCES account(id),
  pan_enc        BYTEA NOT NULL,
  pan_hash       BYTEA NOT NULL UNIQUE,
  bin            VARCHAR(8) NOT NULL,
  pan_last4      CHAR(4) NOT NULL,
  expiry_yymm    CHAR(4) NOT NULL,
  service_code   CHAR(3) NOT NULL DEFAULT '201',
  pvv            CHAR(4),
  pin_try_count  SMALLINT NOT NULL DEFAULT 0,
  pin_try_limit  SMALLINT NOT NULL DEFAULT 3,
  emv_last_atc   INTEGER,
  status         TEXT NOT NULL DEFAULT 'ACTIVE'
                 CHECK (status IN ('ACTIVE','BLOCKED','LOST','STOLEN','EXPIRED','PIN_BLOCKED')),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_card_account ON card(account_id);
-- CVV is never stored; the issuer recomputes it from PAN + expiry + service code via CVK in the HSM.

CREATE TABLE card_limit (
  card_id    BIGINT NOT NULL REFERENCES card(id),
  tran_type  TEXT NOT NULL CHECK (tran_type IN ('ALL','PURCHASE','CASH','ECOM')),
  period     TEXT NOT NULL CHECK (period IN ('PER_TXN','DAILY','MONTHLY')),
  max_amount BIGINT CHECK (max_amount > 0),
  max_count  INTEGER CHECK (max_count > 0),
  PRIMARY KEY (card_id, tran_type, period)
);

CREATE TABLE velocity_counter (
  card_id    BIGINT NOT NULL REFERENCES card(id),
  tran_type  TEXT NOT NULL,
  period     TEXT NOT NULL CHECK (period IN ('DAILY','MONTHLY')),
  period_key TEXT NOT NULL,
  txn_count  INTEGER NOT NULL DEFAULT 0,
  txn_amount BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (card_id, tran_type, period, period_key)
);

CREATE TABLE tran_log (
  id                     BIGINT GENERATED ALWAYS AS IDENTITY,
  business_date          DATE NOT NULL,
  mti                    CHAR(4) NOT NULL,
  tran_type              TEXT NOT NULL CHECK (tran_type IN
                          ('PURCHASE','CASH','REFUND','BALANCE','PREAUTH','COMPLETION','REVERSAL')),
  processing_code        CHAR(6) NOT NULL,
  acquirer_id            VARCHAR(11) NOT NULL REFERENCES acquirer_link(acquirer_id),
  tid                    CHAR(8) NOT NULL,
  mid                    VARCHAR(15) NOT NULL,
  stan                   CHAR(6) NOT NULL,
  transmission_dt_raw    CHAR(10) NOT NULL,
  transmission_at        TIMESTAMPTZ NOT NULL,
  local_tran_at          TIMESTAMP,
  rrn                    CHAR(12) NOT NULL,
  card_id                BIGINT REFERENCES card(id),
  masked_pan             VARCHAR(19),
  pan_hash               BYTEA,
  mcc                    CHAR(4),
  pos_entry_mode         CHAR(3),
  pos_condition_code     CHAR(2),
  amount                 BIGINT NOT NULL CHECK (amount >= 0),
  currency               CHAR(3) NOT NULL,
  approved_amount        BIGINT,
  response_code          CHAR(2) REFERENCES response_code(code),
  auth_code              CHAR(6),
  status                 TEXT NOT NULL CHECK (status IN
                          ('RECEIVED','APPROVED','DECLINED','REVERSED','PARTIALLY_REVERSED',
                           'COMPLETED','EXPIRED')),
  original_tran_id       BIGINT,
  original_business_date DATE,
  original_data_raw      VARCHAR(42),
  is_stand_in            BOOLEAN NOT NULL DEFAULT FALSE,
  decline_reason         TEXT,
  processing_ms          INTEGER,
  masked_message         JSONB,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (id, business_date),
  CONSTRAINT uq_tran_dedupe UNIQUE (acquirer_id, tid, stan, transmission_dt_raw, mti, business_date),
  CONSTRAINT fk_tran_original FOREIGN KEY (original_tran_id, original_business_date)
    REFERENCES tran_log(id, business_date)
) PARTITION BY RANGE (business_date);

-- Lab-only: one wide partition covering the project's lifetime instead of the baseline's
-- month-by-month examples. A monthly partition rotation job is out of scope for this story.
CREATE TABLE tran_log_default PARTITION OF tran_log
  FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');

CREATE INDEX ix_tran_rrn        ON tran_log(rrn);
CREATE INDEX ix_tran_card_time  ON tran_log(card_id, created_at DESC);
CREATE INDEX ix_tran_orig_match ON tran_log(acquirer_id, stan, transmission_dt_raw);

CREATE TABLE auth_hold (
  id                 BIGSERIAL PRIMARY KEY,
  account_id         BIGINT NOT NULL REFERENCES account(id),
  card_id            BIGINT NOT NULL REFERENCES card(id),
  tran_id            BIGINT NOT NULL,
  tran_business_date DATE NOT NULL,
  amount             BIGINT NOT NULL CHECK (amount > 0),
  remaining_amount   BIGINT NOT NULL CHECK (remaining_amount >= 0),
  currency           CHAR(3) NOT NULL,
  status             TEXT NOT NULL DEFAULT 'ACTIVE'
                     CHECK (status IN ('ACTIVE','COMPLETED','RELEASED','EXPIRED')),
  expires_at         TIMESTAMPTZ NOT NULL,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT fk_hold_tran FOREIGN KEY (tran_id, tran_business_date)
    REFERENCES tran_log(id, business_date),
  CONSTRAINT uq_hold_tran UNIQUE (tran_id, tran_business_date)
);
CREATE INDEX ix_hold_expiry ON auth_hold(expires_at) WHERE status = 'ACTIVE';

CREATE TABLE gl_account (
  code TEXT PRIMARY KEY,
  name TEXT NOT NULL
);
INSERT INTO gl_account VALUES
  ('SETTLEMENT_SUSPENSE','Suspense account pending settlement with acquirer'),
  ('FEE_INCOME','Fee income');

CREATE TABLE journal_entry (
  id                 BIGSERIAL PRIMARY KEY,
  tran_id            BIGINT NOT NULL,
  tran_business_date DATE NOT NULL,
  entry_type         TEXT NOT NULL CHECK (entry_type IN
                      ('PURCHASE','CASH','REFUND','REVERSAL','COMPLETION','FEE','ADJUSTMENT')),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT fk_journal_tran FOREIGN KEY (tran_id, tran_business_date)
    REFERENCES tran_log(id, business_date)
);

CREATE TABLE ledger_posting (
  id          BIGSERIAL PRIMARY KEY,
  journal_id  BIGINT NOT NULL REFERENCES journal_entry(id),
  account_id  BIGINT REFERENCES account(id),
  gl_code     TEXT REFERENCES gl_account(code),
  direction   CHAR(1) NOT NULL CHECK (direction IN ('D','C')),
  amount      BIGINT NOT NULL CHECK (amount > 0),
  currency    CHAR(3) NOT NULL,
  CONSTRAINT chk_posting_target CHECK ((account_id IS NULL) <> (gl_code IS NULL))
);
CREATE INDEX ix_posting_journal ON ledger_posting(journal_id);
CREATE INDEX ix_posting_account ON ledger_posting(account_id);

CREATE OR REPLACE FUNCTION check_journal_balanced() RETURNS trigger AS $$
DECLARE diff BIGINT;
BEGIN
  SELECT COALESCE(SUM(CASE direction WHEN 'D' THEN amount ELSE -amount END), 0)
    INTO diff
    FROM ledger_posting
   WHERE journal_id = NEW.journal_id;
  IF diff <> 0 THEN
    RAISE EXCEPTION 'Journal % is not balanced, difference %', NEW.journal_id, diff;
  END IF;
  RETURN NULL;
END $$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_journal_balanced
  AFTER INSERT ON ledger_posting
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION check_journal_balanced();

CREATE TABLE key_store (
  id            BIGSERIAL PRIMARY KEY,
  key_type      TEXT NOT NULL CHECK (key_type IN ('ZMK','ZPK','ZAK','CVK','PVK','IMK_AC')),
  counterparty  VARCHAR(11),
  key_under_lmk TEXT NOT NULL,
  kcv           CHAR(6) NOT NULL,
  status        TEXT NOT NULL CHECK (status IN ('PENDING','ACTIVE','RETIRED')),
  activated_at  TIMESTAMPTZ,
  retired_at    TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_active_key
  ON key_store(key_type, COALESCE(counterparty, ''))
  WHERE status = 'ACTIVE';

CREATE TABLE recon_totals (
  business_date           DATE NOT NULL,
  acquirer_id             VARCHAR(11) NOT NULL REFERENCES acquirer_link(acquirer_id),
  source                  TEXT NOT NULL CHECK (source IN ('ACQUIRER_0500','ISSUER_COMPUTED')),
  credits_count           INTEGER NOT NULL,
  credits_reversal_count  INTEGER NOT NULL,
  debits_count            INTEGER NOT NULL,
  debits_reversal_count   INTEGER NOT NULL,
  credits_amount          BIGINT  NOT NULL,
  credits_reversal_amount BIGINT  NOT NULL,
  debits_amount           BIGINT  NOT NULL,
  debits_reversal_amount  BIGINT  NOT NULL,
  net_amount              BIGINT  NOT NULL,
  result                  TEXT CHECK (result IN ('IN_BALANCE','OUT_OF_BALANCE')),
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (business_date, acquirer_id, source)
);

CREATE TABLE outbox_event (
  id             UUID PRIMARY KEY,
  aggregate_type TEXT NOT NULL CHECK (aggregate_type IN ('TRANSACTION','CARD','CUTOVER')),
  aggregate_id   TEXT NOT NULL,
  event_type     TEXT NOT NULL,
  payload        JSONB NOT NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at   TIMESTAMPTZ,
  attempts       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_outbox_unpublished ON outbox_event(created_at) WHERE published_at IS NULL;

CREATE TABLE audit_log (
  id           BIGSERIAL PRIMARY KEY,
  actor        TEXT NOT NULL,
  action       TEXT NOT NULL,
  entity_type  TEXT NOT NULL,
  entity_id    TEXT NOT NULL,
  before_state JSONB,
  after_state  JSONB,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION forbid_modify() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION '% is an append-only table', TG_TABLE_NAME;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_immutable
  BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION forbid_modify();
