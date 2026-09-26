-- R-1 (#119): a reversal is an advice and can't be declined (docs/03 §7.3), so it must post even
-- when it overdraws the account (a refund reversal after the customer spent the refund). The
-- overdraft floor becomes Authorize's rule for customer-initiated debits, checked under the
-- account's row lock; reversals that end below it are recorded in audit_log
-- (NEGATIVE_BALANCE_AFTER_REVERSAL) for follow-up.
ALTER TABLE account DROP CONSTRAINT chk_available_floor;

-- R-2 (#119): the balance a balance inquiry answered in DE 54, so a duplicate replays it
-- (docs/03 §7.5). Null for every other transaction type.
ALTER TABLE tran_log ADD COLUMN balance BIGINT;
