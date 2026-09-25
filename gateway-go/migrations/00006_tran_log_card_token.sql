-- +goose Up
-- The simulator's card token (never the PAN): a reversal advice rebuilds DE 2 from it at send
-- time (docs/plans/MCN-401-reversal-0420-fix.md ruling 1).
ALTER TABLE tran_log ADD COLUMN card_token TEXT;

-- +goose Down
ALTER TABLE tran_log DROP COLUMN card_token;
