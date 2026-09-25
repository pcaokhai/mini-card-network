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
