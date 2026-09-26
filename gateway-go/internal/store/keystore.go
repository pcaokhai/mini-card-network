package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// KeyRow is one key_store row (docs/assets/baseline-schema.sql:474-488). KeyUnderLMKHex is a
// hex-encoded cryptogram, never the clear key.
type KeyRow struct {
	ID             int64
	KeyType        string
	OwnerRef       string
	KeyUnderLMKHex string
	KCV            string
	Status         string
	ActivatedAt    *time.Time
	RetiredAt      *time.Time
	CreatedAt      time.Time
}

// KeyStoreRepository persists key_store.
type KeyStoreRepository struct{ pool *Pool }

// NewKeyStoreRepository builds a KeyStoreRepository backed by pool.
func NewKeyStoreRepository(pool *Pool) *KeyStoreRepository { return &KeyStoreRepository{pool: pool} }

// Insert records a new PENDING key_store row and returns its id.
func (r *KeyStoreRepository) Insert(ctx context.Context, row KeyRow) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO key_store (key_type, owner_ref, key_under_lmk, kcv, status)
		 VALUES ($1, $2, $3, $4, 'PENDING') RETURNING id`,
		row.KeyType, nullableOwnerRef(row.OwnerRef), row.KeyUnderLMKHex, row.KCV,
	).Scan(&id)
	return id, err
}

// EnsureActive registers an ACTIVE key of keyType (no owner_ref) unless one is already active,
// reporting whether it inserted. The partial unique index uq_acq_active_key makes a concurrent
// second insert a no-op rather than a second ACTIVE key.
func (r *KeyStoreRepository) EnsureActive(ctx context.Context, keyType, keyUnderLMKHex, kcv string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO key_store (key_type, owner_ref, key_under_lmk, kcv, status, activated_at)
		 SELECT $1, NULL, $2, $3, 'ACTIVE', now()
		 WHERE NOT EXISTS (
		   SELECT 1 FROM key_store WHERE key_type = $1 AND owner_ref IS NULL AND status = 'ACTIVE')
		 ON CONFLICT DO NOTHING`,
		keyType, keyUnderLMKHex, kcv)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ActiveKCV returns the KCV of the ACTIVE global key of keyType.
func (r *KeyStoreRepository) ActiveKCV(ctx context.Context, keyType string) (string, error) {
	var kcv string
	err := r.pool.QueryRow(ctx,
		`SELECT kcv FROM key_store WHERE key_type = $1 AND owner_ref IS NULL AND status = 'ACTIVE'`,
		keyType).Scan(&kcv)
	return kcv, err
}

// Activate marks id ACTIVE and retires the prior ACTIVE row for the same (key_type, owner_ref)
// pair, in one transaction (mirrors the issuer's KeyStoreRepository.activate).
func (r *KeyStoreRepository) Activate(ctx context.Context, id int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`UPDATE key_store SET status = 'RETIRED', retired_at = now()
		 WHERE status = 'ACTIVE'
		   AND key_type = (SELECT key_type FROM key_store WHERE id = $1)
		   AND coalesce(owner_ref, '') = (SELECT coalesce(owner_ref, '') FROM key_store WHERE id = $1)`,
		id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE key_store SET status = 'ACTIVE', activated_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// List returns every key_store row, backing GET /v1/keys/acquirer.
func (r *KeyStoreRepository) List(ctx context.Context) ([]KeyRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, key_type, coalesce(owner_ref, ''), key_under_lmk, kcv, status, activated_at, retired_at, created_at
		 FROM key_store ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []KeyRow
	for rows.Next() {
		var row KeyRow
		if err := rows.Scan(&row.ID, &row.KeyType, &row.OwnerRef, &row.KeyUnderLMKHex, &row.KCV,
			&row.Status, &row.ActivatedAt, &row.RetiredAt, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// Get returns the key_store row id.
func (r *KeyStoreRepository) Get(ctx context.Context, id int64) (KeyRow, error) {
	var row KeyRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, key_type, coalesce(owner_ref, ''), key_under_lmk, kcv, status, activated_at, retired_at, created_at
		 FROM key_store WHERE id = $1`, id,
	).Scan(&row.ID, &row.KeyType, &row.OwnerRef, &row.KeyUnderLMKHex, &row.KCV,
		&row.Status, &row.ActivatedAt, &row.RetiredAt, &row.CreatedAt)
	return row, err
}

// ListCurrent returns the keys in use: every ACTIVE row plus a PENDING one while its rotation runs.
// RETIRED rows stay in the table for the dual-key window and the audit trail, but never reach
// GET /v1/keys/acquirer (SEC-G8).
func (r *KeyStoreRepository) ListCurrent(ctx context.Context) ([]KeyRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, key_type, coalesce(owner_ref, ''), key_under_lmk, kcv, status, activated_at, retired_at, created_at
		 FROM key_store WHERE status IN ('ACTIVE', 'PENDING') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []KeyRow
	for rows.Next() {
		var row KeyRow
		if err := rows.Scan(&row.ID, &row.KeyType, &row.OwnerRef, &row.KeyUnderLMKHex, &row.KCV,
			&row.Status, &row.ActivatedAt, &row.RetiredAt, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// RetirePending retires id if it is still PENDING: a rotation that failed after GENERATE must not
// leave its never-activated key listed forever (SEC-G8). An ACTIVE or RETIRED row is untouched.
func (r *KeyStoreRepository) RetirePending(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE key_store SET status = 'RETIRED', retired_at = now() WHERE id = $1 AND status = 'PENDING'`, id)
	return err
}

// FindRecentlyRetired returns the most recently RETIRED row for (keyType, ownerRef) if it
// retired within the last `within` duration, or nil if none qualifies (not an error - "no
// recently-retired key" is the expected steady state outside a rotation's grace window).
func (r *KeyStoreRepository) FindRecentlyRetired(ctx context.Context, keyType, ownerRef string, within time.Duration) (*KeyRow, error) {
	var row KeyRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, key_type, coalesce(owner_ref, ''), key_under_lmk, kcv, status, activated_at, retired_at, created_at
		 FROM key_store
		 WHERE key_type = $1 AND coalesce(owner_ref, '') = $2 AND status = 'RETIRED'
		   AND activated_at IS NOT NULL -- a key retired without ever being used is never a fallback
		   AND retired_at > now() - $3::interval
		 ORDER BY retired_at DESC LIMIT 1`,
		keyType, ownerRef, within.String(),
	).Scan(&row.ID, &row.KeyType, &row.OwnerRef, &row.KeyUnderLMKHex, &row.KCV,
		&row.Status, &row.ActivatedAt, &row.RetiredAt, &row.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// FindPending returns the newest PENDING row of (keyType, ownerRef), or nil if there is none.
func (r *KeyStoreRepository) FindPending(ctx context.Context, keyType, ownerRef string) (*KeyRow, error) {
	var row KeyRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, key_type, coalesce(owner_ref, ''), key_under_lmk, kcv, status, activated_at, retired_at, created_at
		 FROM key_store WHERE key_type = $1 AND coalesce(owner_ref, '') = $2 AND status = 'PENDING'
		 ORDER BY id DESC LIMIT 1`,
		keyType, ownerRef,
	).Scan(&row.ID, &row.KeyType, &row.OwnerRef, &row.KeyUnderLMKHex, &row.KCV,
		&row.Status, &row.ActivatedAt, &row.RetiredAt, &row.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// MACFallbackKeys picks the one extra key a response MAC is retried under. A PENDING key comes
// first: it only survives a rotation whose 0810 never arrived, and the issuer may already MAC
// with it (review S2). Otherwise it is the key retired within the dual-key window (MCN-504-AC2).
// It satisfies purchase.RetiredKeyFinder, hence the method name.
type MACFallbackKeys struct{ repo *KeyStoreRepository }

// MACFallback adapts r for the MAC verifier's single fallback key.
func (r *KeyStoreRepository) MACFallback() MACFallbackKeys { return MACFallbackKeys{repo: r} }

// FindRecentlyRetired returns the pending key of (keyType, ownerRef) if any, else the key retired
// within the window, else nil.
func (f MACFallbackKeys) FindRecentlyRetired(ctx context.Context, keyType, ownerRef string, within time.Duration) (*KeyRow, error) {
	pending, err := f.repo.FindPending(ctx, keyType, ownerRef)
	if err != nil || pending != nil {
		return pending, err
	}
	return f.repo.FindRecentlyRetired(ctx, keyType, ownerRef, within)
}

func nullableOwnerRef(ownerRef string) any {
	if ownerRef == "" {
		return nil
	}
	return ownerRef
}
