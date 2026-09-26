-- +goose Up
-- The RRN a reserved key sent, so a retry of a request whose first attempt crashed or failed after
-- the send can answer from tran_log instead of waiting out the key (#117 review S1).
ALTER TABLE idempotency_record ADD COLUMN rrn CHAR(12);
-- Fences a reservation: a holder whose key was reclaimed (it was slow, not dead) can neither record
-- an RRN, and so never sends, nor release the new holder's reservation (#117 re-review).
ALTER TABLE idempotency_record ADD COLUMN reservation_token UUID;
-- The completion that consumed a pre-authorization's hold: a pre-auth is completed at most once
-- (#117 review S2).
ALTER TABLE tran_log ADD COLUMN completed_by CHAR(12);

-- +goose Down
ALTER TABLE tran_log DROP COLUMN completed_by;
ALTER TABLE idempotency_record DROP COLUMN reservation_token;
ALTER TABLE idempotency_record DROP COLUMN rrn;
