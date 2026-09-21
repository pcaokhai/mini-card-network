-- =====================================================================
-- Mini Card Network — Database schema (PostgreSQL 16)
--
-- Ba schema tách biệt: issuer, acquirer, settlement.
-- Ngoài thực tế đây là 3 tổ chức với 3 database riêng; trong lab bạn có thể
-- chạy 3 database hoặc 3 schema trên cùng một instance cho tiện.
--
-- Quy ước chung:
--   * Tiền lưu minor units (BIGINT). VND exponent = 0 nên 1 đơn vị = 1 đồng.
--   * Currency: mã số ISO 4217 dạng CHAR(3), ví dụ '704' = VND, '840' = USD.
--   * Trạng thái dùng TEXT + CHECK thay cho ENUM để migrate dễ hơn.
--   * PCI DSS: KHÔNG lưu track 2, CVV/CVC, PIN hay PIN block sau authorize.
--     PAN chỉ lưu dạng mã hóa + HMAC để tra cứu + dạng mask để hiển thị.
-- =====================================================================

CREATE SCHEMA IF NOT EXISTS issuer;
CREATE SCHEMA IF NOT EXISTS acquirer;
CREATE SCHEMA IF NOT EXISTS settlement;


-- #####################################################################
-- ISSUER
-- #####################################################################

-- ---------- Reference & trạng thái hệ thống ----------

CREATE TABLE issuer.response_code (
  code        CHAR(2) PRIMARY KEY,                -- field 39
  description TEXT NOT NULL,
  category    TEXT NOT NULL CHECK (category IN ('APPROVED','DECLINED','ERROR','PICKUP'))
);

INSERT INTO issuer.response_code VALUES
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

-- Một dòng duy nhất giữ business date hiện hành
CREATE TABLE issuer.system_state (
  id                    SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  current_business_date DATE NOT NULL,
  cutover_in_progress   BOOLEAN NOT NULL DEFAULT FALSE,
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE issuer.cutover_log (
  id                 BIGSERIAL PRIMARY KEY,
  from_business_date DATE NOT NULL,
  to_business_date   DATE NOT NULL,
  triggered_by       TEXT NOT NULL CHECK (triggered_by IN ('SCHEDULER','NETWORK_0800','MANUAL')),
  started_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at        TIMESTAMPTZ
);

-- Phiên kết nối với từng acquirer (sign-on, echo)
CREATE TABLE issuer.acquirer_link (
  acquirer_id   VARCHAR(11) PRIMARY KEY,           -- field 32
  name          TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'SIGNED_OFF'
                CHECK (status IN ('SIGNED_ON','SIGNED_OFF','SUSPENDED')),
  signed_on_at  TIMESTAMPTZ,
  last_echo_at  TIMESTAMPTZ,
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------- Tài khoản & thẻ ----------

CREATE TABLE issuer.account (
  id                BIGSERIAL PRIMARY KEY,
  account_no        VARCHAR(20) NOT NULL UNIQUE,
  currency          CHAR(3) NOT NULL,
  ledger_balance    BIGINT NOT NULL DEFAULT 0,     -- số dư sổ sách (đã hạch toán)
  available_balance BIGINT NOT NULL DEFAULT 0,     -- = ledger - hold đang ACTIVE
  overdraft_limit   BIGINT NOT NULL DEFAULT 0 CHECK (overdraft_limit >= 0),
  status            TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','FROZEN','CLOSED')),
  version           BIGINT NOT NULL DEFAULT 0,     -- optimistic locking
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- Chốt chặn cuối cùng ở tầng DB: kể cả code có bug cũng không âm quá hạn mức
  CONSTRAINT chk_available_floor CHECK (available_balance >= -overdraft_limit)
);

CREATE TABLE issuer.card (
  id             BIGSERIAL PRIMARY KEY,
  account_id     BIGINT NOT NULL REFERENCES issuer.account(id),
  pan_enc        BYTEA NOT NULL,                   -- AES-GCM, DEK được bọc bởi KEK
  pan_hash       BYTEA NOT NULL UNIQUE,            -- HMAC-SHA256(PAN): tra cứu không cần giải mã
  bin            VARCHAR(8) NOT NULL,
  pan_last4      CHAR(4) NOT NULL,
  expiry_yymm    CHAR(4) NOT NULL,                 -- so với field 14
  service_code   CHAR(3) NOT NULL DEFAULT '201',
  pvv            CHAR(4),                          -- PIN verification value, KHÔNG lưu PIN
  pin_try_count  SMALLINT NOT NULL DEFAULT 0,
  pin_try_limit  SMALLINT NOT NULL DEFAULT 3,
  emv_last_atc   INTEGER,                          -- ATC gần nhất, chống replay cryptogram
  status         TEXT NOT NULL DEFAULT 'ACTIVE'
                 CHECK (status IN ('ACTIVE','BLOCKED','LOST','STOLEN','EXPIRED','PIN_BLOCKED')),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_card_account ON issuer.card(account_id);
-- Lưu ý: CVV không lưu. Issuer TÍNH LẠI CVV từ PAN + expiry + service code bằng CVK trong HSM.

CREATE TABLE issuer.card_limit (
  card_id    BIGINT NOT NULL REFERENCES issuer.card(id),
  tran_type  TEXT NOT NULL CHECK (tran_type IN ('ALL','PURCHASE','CASH','ECOM')),
  period     TEXT NOT NULL CHECK (period IN ('PER_TXN','DAILY','MONTHLY')),
  max_amount BIGINT CHECK (max_amount > 0),
  max_count  INTEGER CHECK (max_count > 0),
  PRIMARY KEY (card_id, tran_type, period)
);

-- Bộ đếm cập nhật trong cùng DB transaction với authorize (nhanh hơn SUM trên tran_log)
CREATE TABLE issuer.velocity_counter (
  card_id    BIGINT NOT NULL REFERENCES issuer.card(id),
  tran_type  TEXT NOT NULL,
  period     TEXT NOT NULL CHECK (period IN ('DAILY','MONTHLY')),
  period_key TEXT NOT NULL,                        -- '2026-09-21' hoặc '2026-09'
  txn_count  INTEGER NOT NULL DEFAULT 0,
  txn_amount BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (card_id, tran_type, period, period_key)
);

-- ---------- Transaction log (partition theo business_date) ----------

CREATE TABLE issuer.tran_log (
  id                     BIGINT GENERATED ALWAYS AS IDENTITY,
  business_date          DATE NOT NULL,            -- field 15 sau cutover
  mti                    CHAR(4) NOT NULL,         -- repeat (x21) được chuẩn hóa về x20 trước khi ghi
  tran_type              TEXT NOT NULL CHECK (tran_type IN
                          ('PURCHASE','CASH','REFUND','BALANCE','PREAUTH','COMPLETION','REVERSAL')),
  processing_code        CHAR(6) NOT NULL,         -- field 3
  acquirer_id            VARCHAR(11) NOT NULL REFERENCES issuer.acquirer_link(acquirer_id), -- field 32
  tid                    CHAR(8) NOT NULL,         -- field 41
  mid                    VARCHAR(15) NOT NULL,     -- field 42
  stan                   CHAR(6) NOT NULL,         -- field 11
  transmission_dt_raw    CHAR(10) NOT NULL,        -- field 7 MMDDhhmmss (GMT, KHÔNG có năm)
  transmission_at        TIMESTAMPTZ NOT NULL,     -- field 7 sau khi suy ra năm
  local_tran_at          TIMESTAMP,                -- field 12 + 13
  rrn                    CHAR(12) NOT NULL,        -- field 37
  card_id                BIGINT REFERENCES issuer.card(id), -- NULL nếu thẻ không tồn tại (RC 14)
  masked_pan             VARCHAR(19),              -- 6 số đầu + 4 số cuối
  pan_hash               BYTEA,
  mcc                    CHAR(4),                  -- field 18
  pos_entry_mode         CHAR(3),                  -- field 22
  pos_condition_code     CHAR(2),                  -- field 25
  amount                 BIGINT NOT NULL CHECK (amount >= 0), -- field 4
  currency               CHAR(3) NOT NULL,         -- field 49
  approved_amount        BIGINT,                   -- partial approval
  response_code          CHAR(2) REFERENCES issuer.response_code(code), -- field 39
  auth_code              CHAR(6),                  -- field 38
  status                 TEXT NOT NULL CHECK (status IN
                          ('RECEIVED','APPROVED','DECLINED','REVERSED','PARTIALLY_REVERSED',
                           'COMPLETED','EXPIRED')),
  original_tran_id       BIGINT,                   -- reversal/completion trỏ về giao dịch gốc
  original_business_date DATE,
  original_data_raw      VARCHAR(42),              -- field 90 nguyên bản
  is_stand_in            BOOLEAN NOT NULL DEFAULT FALSE,
  decline_reason         TEXT,                     -- lý do nội bộ, không gửi ra ngoài
  processing_ms          INTEGER,
  masked_message         JSONB,                    -- message đã mask field 2/35/52/55 để debug
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (id, business_date),
  -- Chống trùng trong phạm vi business date: cùng acquirer + terminal + STAN + field 7 + MTI
  CONSTRAINT uq_tran_dedupe UNIQUE (acquirer_id, tid, stan, transmission_dt_raw, mti, business_date),
  CONSTRAINT fk_tran_original FOREIGN KEY (original_tran_id, original_business_date)
    REFERENCES issuer.tran_log(id, business_date)
) PARTITION BY RANGE (business_date);

-- Mỗi tháng một partition; production thường dùng pg_partman để tạo tự động
CREATE TABLE issuer.tran_log_2026_09 PARTITION OF issuer.tran_log
  FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE issuer.tran_log_2026_10 PARTITION OF issuer.tran_log
  FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE INDEX ix_tran_rrn        ON issuer.tran_log(rrn);
CREATE INDEX ix_tran_card_time  ON issuer.tran_log(card_id, created_at DESC);
-- Tìm giao dịch gốc khi nhận reversal (field 90 chứa STAN + field 7 + acquirer gốc)
CREATE INDEX ix_tran_orig_match ON issuer.tran_log(acquirer_id, stan, transmission_dt_raw);

-- ---------- Auth hold (pre-auth, và hold tạm trong 0100) ----------

CREATE TABLE issuer.auth_hold (
  id                 BIGSERIAL PRIMARY KEY,
  account_id         BIGINT NOT NULL REFERENCES issuer.account(id),
  card_id            BIGINT NOT NULL REFERENCES issuer.card(id),
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
    REFERENCES issuer.tran_log(id, business_date),
  CONSTRAINT uq_hold_tran UNIQUE (tran_id, tran_business_date)
);
-- Job giải phóng hold hết hạn chỉ quét hold còn ACTIVE
CREATE INDEX ix_hold_expiry ON issuer.auth_hold(expires_at) WHERE status = 'ACTIVE';

-- ---------- Sổ cái kép (double-entry) ----------

CREATE TABLE issuer.gl_account (
  code TEXT PRIMARY KEY,                           -- 'SETTLEMENT_SUSPENSE', 'FEE_INCOME', ...
  name TEXT NOT NULL
);
INSERT INTO issuer.gl_account VALUES
  ('SETTLEMENT_SUSPENSE','Tài khoản treo chờ quyết toán với acquirer'),
  ('FEE_INCOME','Thu phí');

CREATE TABLE issuer.journal_entry (
  id                 BIGSERIAL PRIMARY KEY,
  tran_id            BIGINT NOT NULL,
  tran_business_date DATE NOT NULL,
  entry_type         TEXT NOT NULL CHECK (entry_type IN
                      ('PURCHASE','CASH','REFUND','REVERSAL','COMPLETION','FEE','ADJUSTMENT')),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT fk_journal_tran FOREIGN KEY (tran_id, tran_business_date)
    REFERENCES issuer.tran_log(id, business_date)
);

CREATE TABLE issuer.ledger_posting (
  id          BIGSERIAL PRIMARY KEY,
  journal_id  BIGINT NOT NULL REFERENCES issuer.journal_entry(id),
  account_id  BIGINT REFERENCES issuer.account(id),     -- tài khoản khách hàng
  gl_code     TEXT REFERENCES issuer.gl_account(code),  -- hoặc tài khoản nội bộ
  direction   CHAR(1) NOT NULL CHECK (direction IN ('D','C')),
  amount      BIGINT NOT NULL CHECK (amount > 0),
  currency    CHAR(3) NOT NULL,
  CONSTRAINT chk_posting_target CHECK ((account_id IS NULL) <> (gl_code IS NULL))
);
CREATE INDEX ix_posting_journal ON issuer.ledger_posting(journal_id);
CREATE INDEX ix_posting_account ON issuer.ledger_posting(account_id);

-- Mỗi journal phải cân (tổng Nợ = tổng Có), kiểm tra lúc COMMIT
CREATE OR REPLACE FUNCTION issuer.check_journal_balanced() RETURNS trigger AS $$
DECLARE diff BIGINT;
BEGIN
  SELECT COALESCE(SUM(CASE direction WHEN 'D' THEN amount ELSE -amount END), 0)
    INTO diff
    FROM issuer.ledger_posting
   WHERE journal_id = NEW.journal_id;
  IF diff <> 0 THEN
    RAISE EXCEPTION 'Journal % không cân, chênh lệch %', NEW.journal_id, diff;
  END IF;
  RETURN NULL;
END $$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_journal_balanced
  AFTER INSERT ON issuer.ledger_posting
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION issuer.check_journal_balanced();

-- ---------- Key management ----------

CREATE TABLE issuer.key_store (
  id            BIGSERIAL PRIMARY KEY,
  key_type      TEXT NOT NULL CHECK (key_type IN ('ZMK','ZPK','ZAK','CVK','PVK','IMK_AC')),
  counterparty  VARCHAR(11),                       -- acquirer_id; NULL với key nội bộ
  key_under_lmk TEXT NOT NULL,                     -- cryptogram dưới LMK, KHÔNG BAO GIỜ là key rõ
  kcv           CHAR(6) NOT NULL,                  -- key check value để đối chiếu
  status        TEXT NOT NULL CHECK (status IN ('PENDING','ACTIVE','RETIRED')),
  activated_at  TIMESTAMPTZ,
  retired_at    TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Tại một thời điểm chỉ có một key ACTIVE cho mỗi loại + đối tác
CREATE UNIQUE INDEX uq_active_key
  ON issuer.key_store(key_type, COALESCE(counterparty, ''))
  WHERE status = 'ACTIVE';

-- ---------- Đối soát 0500 ----------

CREATE TABLE issuer.recon_totals (
  business_date           DATE NOT NULL,
  acquirer_id             VARCHAR(11) NOT NULL REFERENCES issuer.acquirer_link(acquirer_id),
  source                  TEXT NOT NULL CHECK (source IN ('ACQUIRER_0500','ISSUER_COMPUTED')),
  credits_count           INTEGER NOT NULL,      -- field 74
  credits_reversal_count  INTEGER NOT NULL,      -- field 75
  debits_count            INTEGER NOT NULL,      -- field 76
  debits_reversal_count   INTEGER NOT NULL,      -- field 77
  credits_amount          BIGINT  NOT NULL,      -- field 86
  credits_reversal_amount BIGINT  NOT NULL,      -- field 87
  debits_amount           BIGINT  NOT NULL,      -- field 88
  debits_reversal_amount  BIGINT  NOT NULL,      -- field 89
  net_amount              BIGINT  NOT NULL,      -- field 97
  result                  TEXT CHECK (result IN ('IN_BALANCE','OUT_OF_BALANCE')),
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (business_date, acquirer_id, source)
);

-- ---------- Outbox (đẩy sự kiện sang Kafka) ----------

CREATE TABLE issuer.outbox_event (
  id             UUID PRIMARY KEY,
  aggregate_type TEXT NOT NULL CHECK (aggregate_type IN ('TRANSACTION','CARD','CUTOVER')),
  aggregate_id   TEXT NOT NULL,                    -- dùng làm Kafka key để giữ thứ tự theo thẻ
  event_type     TEXT NOT NULL,                    -- TransactionApproved, TransactionReversed, ...
  payload        JSONB NOT NULL,                   -- không chứa PAN rõ
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at   TIMESTAMPTZ,
  attempts       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_outbox_unpublished ON issuer.outbox_event(created_at) WHERE published_at IS NULL;

-- ---------- Audit (append-only) ----------

CREATE TABLE issuer.audit_log (
  id           BIGSERIAL PRIMARY KEY,
  actor        TEXT NOT NULL,                      -- user vận hành hoặc tên service
  action       TEXT NOT NULL,                      -- CARD_BLOCKED, KEY_ROTATED, LIMIT_CHANGED...
  entity_type  TEXT NOT NULL,
  entity_id    TEXT NOT NULL,
  before_state JSONB,
  after_state  JSONB,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION issuer.forbid_modify() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION '% là bảng append-only', TG_TABLE_NAME;
END $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_immutable
  BEFORE UPDATE OR DELETE ON issuer.audit_log
  FOR EACH ROW EXECUTE FUNCTION issuer.forbid_modify();


-- #####################################################################
-- ACQUIRER (Go gateway)
-- #####################################################################

CREATE TABLE acquirer.merchant (
  mid        VARCHAR(15) PRIMARY KEY,              -- field 42
  name       TEXT NOT NULL,                        -- dùng cho field 43
  mcc        CHAR(4) NOT NULL,                     -- field 18
  city       TEXT,
  country    CHAR(2) NOT NULL DEFAULT 'VN',
  status     TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUSPENDED','CLOSED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE acquirer.terminal (
  tid          CHAR(8) PRIMARY KEY,                -- field 41
  mid          VARCHAR(15) NOT NULL REFERENCES acquirer.merchant(mid),
  status       TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','DISABLED')),
  last_tstan   INTEGER NOT NULL DEFAULT 0 CHECK (last_tstan BETWEEN 0 AND 999999), -- STAN phía terminal
  batch_no     INTEGER NOT NULL DEFAULT 1,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- STAN phía mạng (gateway -> issuer): 000001..999999 rồi vòng lại
CREATE SEQUENCE acquirer.network_stan_seq MINVALUE 1 MAXVALUE 999999 CYCLE;
-- RRN (field 37) gợi ý định dạng: Y + DDD (ngày trong năm) + hh + STAN 6 số = 12 ký tự

CREATE TABLE acquirer.tran_log (
  id                     BIGINT GENERATED ALWAYS AS IDENTITY,
  business_date          DATE NOT NULL,
  client_request_id      UUID NOT NULL,            -- idempotency key từ Next.js
  tran_type              TEXT NOT NULL,
  mti                    CHAR(4) NOT NULL,
  processing_code        CHAR(6) NOT NULL,
  tid                    CHAR(8) NOT NULL REFERENCES acquirer.terminal(tid),
  mid                    VARCHAR(15) NOT NULL,
  terminal_stan          CHAR(6),
  network_stan           CHAR(6) NOT NULL,         -- field 11 gửi sang issuer
  rrn                    CHAR(12) NOT NULL,
  transmission_dt_raw    CHAR(10) NOT NULL,
  transmission_at        TIMESTAMPTZ NOT NULL,
  masked_pan             VARCHAR(19),
  pan_hash               BYTEA,
  amount                 BIGINT NOT NULL,
  currency               CHAR(3) NOT NULL,
  pos_entry_mode         CHAR(3),
  -- State machine: CREATED -> SENT -> APPROVED | DECLINED | TIMED_OUT
  --                TIMED_OUT -> REVERSAL_PENDING -> REVERSED
  state                  TEXT NOT NULL CHECK (state IN
                          ('CREATED','SENT','APPROVED','DECLINED','TIMED_OUT',
                           'REVERSAL_PENDING','REVERSED','FAILED')),
  response_code          CHAR(2),
  auth_code              CHAR(6),
  late_response_code     CHAR(2),                  -- response đến sau khi đã timeout
  late_response_at       TIMESTAMPTZ,
  original_tran_id       BIGINT,
  original_business_date DATE,
  sent_at                TIMESTAMPTZ,
  responded_at           TIMESTAMPTZ,
  latency_ms             INTEGER,
  trace_id               VARCHAR(32),              -- OpenTelemetry trace id
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (id, business_date),
  CONSTRAINT uq_acq_client_req UNIQUE (client_request_id, business_date),
  CONSTRAINT uq_acq_network_key UNIQUE (network_stan, transmission_dt_raw, business_date),
  CONSTRAINT fk_acq_original FOREIGN KEY (original_tran_id, original_business_date)
    REFERENCES acquirer.tran_log(id, business_date)
) PARTITION BY RANGE (business_date);

CREATE TABLE acquirer.tran_log_2026_09 PARTITION OF acquirer.tran_log
  FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE acquirer.tran_log_2026_10 PARTITION OF acquirer.tran_log
  FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE INDEX ix_acq_rrn   ON acquirer.tran_log(rrn);
CREATE INDEX ix_acq_state ON acquirer.tran_log(state) WHERE state IN ('SENT','TIMED_OUT','REVERSAL_PENDING');

-- Lịch sử chuyển trạng thái: debug và audit state machine
CREATE TABLE acquirer.tran_state_history (
  id                 BIGSERIAL PRIMARY KEY,
  tran_id            BIGINT NOT NULL,
  tran_business_date DATE NOT NULL,
  from_state         TEXT,
  to_state           TEXT NOT NULL,
  reason             TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT fk_hist_tran FOREIGN KEY (tran_id, tran_business_date)
    REFERENCES acquirer.tran_log(id, business_date)
);

-- Store-and-forward cho advice (0120, 0220, 0420)
CREATE TABLE acquirer.saf_queue (
  id                 BIGSERIAL PRIMARY KEY,
  tran_id            BIGINT NOT NULL,
  tran_business_date DATE NOT NULL,
  mti                CHAR(4) NOT NULL CHECK (mti IN ('0120','0220','0420')),
  payload_enc        BYTEA NOT NULL,              -- message mã hóa vì có thể chứa PAN
  status             TEXT NOT NULL DEFAULT 'PENDING'
                     CHECK (status IN ('PENDING','IN_FLIGHT','ACKED','DEAD')),
  attempts           INTEGER NOT NULL DEFAULT 0,  -- attempts > 0 thì gửi dạng repeat (x21)
  max_attempts       INTEGER NOT NULL DEFAULT 20,
  next_retry_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error         TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  acked_at           TIMESTAMPTZ,
  CONSTRAINT fk_saf_tran FOREIGN KEY (tran_id, tran_business_date)
    REFERENCES acquirer.tran_log(id, business_date)
);
CREATE INDEX ix_saf_due ON acquirer.saf_queue(next_retry_at) WHERE status = 'PENDING';
-- Worker lấy việc an toàn khi chạy nhiều instance:
--   SELECT * FROM acquirer.saf_queue
--    WHERE status = 'PENDING' AND next_retry_at <= now()
--    ORDER BY id
--    FOR UPDATE SKIP LOCKED
--    LIMIT 50;

CREATE TABLE acquirer.link_state (
  endpoint                  TEXT PRIMARY KEY,    -- 'issuer-a:8000'
  status                    TEXT NOT NULL CHECK (status IN ('DISCONNECTED','CONNECTED','SIGNED_ON')),
  signed_on_at              TIMESTAMPTZ,
  last_echo_ok_at           TIMESTAMPTZ,
  consecutive_echo_failures INTEGER NOT NULL DEFAULT 0,
  updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE acquirer.key_store (
  id            BIGSERIAL PRIMARY KEY,
  key_type      TEXT NOT NULL CHECK (key_type IN ('ZMK','ZPK','ZAK','TPK','TAK')),
  owner_ref     TEXT,                              -- tid với TPK/TAK, endpoint với ZPK/ZAK
  key_under_lmk TEXT NOT NULL,
  kcv           CHAR(6) NOT NULL,
  status        TEXT NOT NULL CHECK (status IN ('PENDING','ACTIVE','RETIRED')),
  activated_at  TIMESTAMPTZ,
  retired_at    TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_acq_active_key
  ON acquirer.key_store(key_type, COALESCE(owner_ref, ''))
  WHERE status = 'ACTIVE';

CREATE TABLE acquirer.settlement_batch (
  business_date     DATE PRIMARY KEY,
  status            TEXT NOT NULL CHECK (status IN
                     ('OPEN','CUTOVER','RECON_SENT','IN_BALANCE','OUT_OF_BALANCE')),
  totals            JSONB,                         -- ánh xạ field 74..89, 97 của 0500
  recon_sent_at     TIMESTAMPTZ,
  recon_result_code CHAR(2),                       -- field 39 của 0510
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);


-- #####################################################################
-- SETTLEMENT (Spring Boot, consume Kafka)
-- #####################################################################

-- Inbox pattern: consumer idempotent, bỏ qua event đã xử lý
CREATE TABLE settlement.inbox_event (
  event_id     UUID PRIMARY KEY,
  source       TEXT NOT NULL CHECK (source IN ('ACQUIRER','ISSUER')),
  event_type   TEXT NOT NULL,
  received_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  processed_at TIMESTAMPTZ
);

-- Bản ghi clearing đã "chốt" (sau khi áp reversal) từ mỗi phía
CREATE TABLE settlement.clearing_record (
  id                  BIGSERIAL PRIMARY KEY,
  source              TEXT NOT NULL CHECK (source IN ('ACQUIRER','ISSUER')),
  business_date       DATE NOT NULL,
  acquirer_id         VARCHAR(11) NOT NULL,
  tid                 CHAR(8) NOT NULL,
  stan                CHAR(6) NOT NULL,
  transmission_dt_raw CHAR(10) NOT NULL,
  rrn                 CHAR(12) NOT NULL,
  tran_type           TEXT NOT NULL,
  amount              BIGINT NOT NULL,
  currency            CHAR(3) NOT NULL,
  final_status        TEXT NOT NULL CHECK (final_status IN ('SETTLE','REVERSED','DECLINED')),
  auth_code           CHAR(6),
  source_tran_id      BIGINT NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_clearing UNIQUE (source, business_date, acquirer_id, tid, stan, transmission_dt_raw)
);
CREATE INDEX ix_clearing_match ON settlement.clearing_record(business_date, rrn);

CREATE TABLE settlement.recon_run (
  id            BIGSERIAL PRIMARY KEY,
  business_date DATE NOT NULL,
  status        TEXT NOT NULL CHECK (status IN ('RUNNING','COMPLETED','FAILED')),
  matched_count INTEGER NOT NULL DEFAULT 0,
  break_count   INTEGER NOT NULL DEFAULT 0,
  started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at   TIMESTAMPTZ
);

CREATE TABLE settlement.recon_break (
  id                 BIGSERIAL PRIMARY KEY,
  run_id             BIGINT NOT NULL REFERENCES settlement.recon_run(id),
  break_type         TEXT NOT NULL CHECK (break_type IN
                      ('MISSING_AT_ISSUER','MISSING_AT_ACQUIRER','AMOUNT_MISMATCH',
                       'STATUS_MISMATCH','DUPLICATE')),
  acquirer_record_id BIGINT REFERENCES settlement.clearing_record(id),
  issuer_record_id   BIGINT REFERENCES settlement.clearing_record(id),
  amount_diff        BIGINT,
  resolution         TEXT NOT NULL DEFAULT 'OPEN'
                     CHECK (resolution IN ('OPEN','AUTO_RESOLVED','MANUAL_ADJUSTED','WRITTEN_OFF')),
  note               TEXT,
  resolved_by        TEXT,
  resolved_at        TIMESTAMPTZ,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_break_open ON settlement.recon_break(run_id) WHERE resolution = 'OPEN';

CREATE TABLE settlement.net_position (
  business_date  DATE NOT NULL,
  participant_id VARCHAR(11) NOT NULL,
  currency       CHAR(3) NOT NULL,
  gross_debit    BIGINT NOT NULL DEFAULT 0,
  gross_credit   BIGINT NOT NULL DEFAULT 0,
  net_amount     BIGINT NOT NULL,                  -- dương: nhận tiền, âm: phải trả
  PRIMARY KEY (business_date, participant_id, currency)
);

CREATE TABLE settlement.clearing_file (
  id              BIGSERIAL PRIMARY KEY,
  business_date   DATE NOT NULL,
  file_name       TEXT NOT NULL UNIQUE,
  record_count    INTEGER NOT NULL,
  total_amount    BIGINT NOT NULL,
  checksum_sha256 CHAR(64) NOT NULL,               -- trailer + checksum chống sửa file
  status          TEXT NOT NULL CHECK (status IN ('GENERATED','SENT','ACKNOWLEDGED','REJECTED')),
  generated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
