-- +goose Up
-- What the Transaction detail and the journey report beyond the request (JRN-G3), the request's
-- W3C trace id (POS-G8), and the DE 39 reason of the 0420 queued for it (JRN-G7).
ALTER TABLE tran_log
  ADD COLUMN approved_amount  BIGINT,
  ADD COLUMN balance_amount   BIGINT,
  ADD COLUMN balance_currency CHAR(3),
  ADD COLUMN original_rrn     CHAR(12),
  ADD COLUMN trace_id         VARCHAR(32),
  ADD COLUMN reversal_reason  CHAR(2);

CREATE INDEX ix_acq_reversal_reason ON tran_log(reversal_reason) WHERE reversal_reason IS NOT NULL;

-- +goose Down
DROP INDEX ix_acq_reversal_reason;
ALTER TABLE tran_log
  DROP COLUMN reversal_reason,
  DROP COLUMN trace_id,
  DROP COLUMN original_rrn,
  DROP COLUMN balance_currency,
  DROP COLUMN balance_amount,
  DROP COLUMN approved_amount;
