-- +goose Up
-- Review S2 of #121: link a rotation to the key it generated, so startup recovery can tell a
-- rotation that never produced a key from one whose outcome at the issuer is unknown.
ALTER TABLE key_rotation ADD COLUMN new_key_id BIGINT REFERENCES key_store(id);

-- +goose Down
ALTER TABLE key_rotation DROP COLUMN new_key_id;
