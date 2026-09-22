-- Reconciles MCN-201's already-shipped acquirer_link with the baseline schema. Keeps MCN-201's
-- table/status vocabulary (DISCONNECTED/CONNECTED/SIGNED_ON/DOWN) since NetworkManagementListener
-- already writes it in production; only adds the baseline's `name` column and renames
-- last_sign_on_at -> signed_on_at to match the baseline's naming. See docs/plans/MCN-301.md Ruling.
ALTER TABLE acquirer_link ADD COLUMN name TEXT NOT NULL DEFAULT 'Lab Acquirer';
ALTER TABLE acquirer_link RENAME COLUMN last_sign_on_at TO signed_on_at;
