-- MCN-402: a reversal whose original transaction cannot be located is recorded here so a
-- later-arriving original for the same key is declined RC 94 instead of posted (docs/03 §7.3).
CREATE TABLE reversal_without_original (
  id                 BIGSERIAL PRIMARY KEY,
  business_date      DATE NOT NULL,
  original_mti       CHAR(4) NOT NULL,
  original_stan      CHAR(6) NOT NULL,
  original_de7       CHAR(10) NOT NULL,
  original_acquirer  CHAR(11) NOT NULL,
  received_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT uq_rwo_key UNIQUE (original_mti, original_stan, original_de7, original_acquirer)
);
