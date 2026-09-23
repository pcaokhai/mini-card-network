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
