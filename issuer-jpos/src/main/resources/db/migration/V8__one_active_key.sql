-- SEC-G15: VerifySecurity and Respond read the ACTIVE ZAK/ZPK from key_store and seed it from the
-- environment on first start. At most one ACTIVE key per (type, counterparty), so two beans
-- seeding at the same startup can't both insert one (the second INSERT hits this index and does
-- nothing). KeyStoreRepository.activate retires the old row before activating the new one, in one
-- transaction, so a rotation never trips it. V7 is reserved by #119.
CREATE UNIQUE INDEX ux_key_store_one_active
  ON key_store (key_type, COALESCE(counterparty, ''))
  WHERE status = 'ACTIVE';
