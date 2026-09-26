-- +goose Up
-- NET-G15: a language-neutral code per event (contracts NetworkEventCode). Nullable: rows written
-- before this migration have none.
ALTER TABLE network_event ADD COLUMN code TEXT;

-- +goose Down
ALTER TABLE network_event DROP COLUMN code;
