package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrUnknownTerminal means a terminal ID has no row in the terminal table, so no merchant can
// be attributed to a transaction from it.
var ErrUnknownTerminal = errors.New("unknown terminal")

// Merchant is the merchant a terminal belongs to: MID goes out as ISO DE 42, Name is what the
// console shows.
type Merchant struct {
	MID  string
	Name string
}

// TerminalRepository resolves terminals to their merchant (contracts/fixtures/cards.json
// `terminals`, loaded by the seed).
type TerminalRepository struct{ pool *Pool }

// NewTerminalRepository returns a TerminalRepository over pool.
func NewTerminalRepository(pool *Pool) *TerminalRepository { return &TerminalRepository{pool: pool} }

// Merchant returns the merchant owning tid, or ErrUnknownTerminal.
func (r *TerminalRepository) Merchant(ctx context.Context, tid string) (Merchant, error) {
	var m Merchant
	err := r.pool.QueryRow(ctx,
		`SELECT m.mid, m.name FROM terminal t JOIN merchant m ON m.mid = t.mid WHERE t.tid = $1`, tid,
	).Scan(&m.MID, &m.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Merchant{}, ErrUnknownTerminal
	}
	return m, err
}

// FixtureTerminal is one contracts/fixtures/cards.json terminal with its merchant.
type FixtureTerminal struct {
	TerminalID   string
	MerchantID   string
	MerchantName string
	MCC          string
}

// List returns every terminal with its merchant, backing GET /v1/terminals (NET-G13).
func (r *TerminalRepository) List(ctx context.Context) ([]FixtureTerminal, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT t.tid, m.mid, m.name, m.mcc FROM terminal t JOIN merchant m ON m.mid = t.mid ORDER BY t.tid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var terminals []FixtureTerminal
	for rows.Next() {
		var t FixtureTerminal
		if err := rows.Scan(&t.TerminalID, &t.MerchantID, &t.MerchantName, &t.MCC); err != nil {
			return nil, err
		}
		terminals = append(terminals, t)
	}
	return terminals, rows.Err()
}

// UpsertFromFixture makes the merchant and terminal tables match the fixture: rows are added, and
// an existing merchant's name and MCC follow the fixture. Rows the fixture no longer lists stay,
// since tran_log references them.
func (r *TerminalRepository) UpsertFromFixture(ctx context.Context, terminals []FixtureTerminal) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, t := range terminals {
		if _, err := tx.Exec(ctx,
			`INSERT INTO merchant (mid, name, mcc) VALUES ($1, $2, $3)
			 ON CONFLICT (mid) DO UPDATE SET name = EXCLUDED.name, mcc = EXCLUDED.mcc`,
			t.MerchantID, t.MerchantName, t.MCC); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO terminal (tid, mid) VALUES ($1, $2) ON CONFLICT (tid) DO UPDATE SET mid = EXCLUDED.mid`,
			t.TerminalID, t.MerchantID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
