package saf

import (
	"context"
	"errors"
	"fmt"

	"github.com/mcn/gateway-go/internal/journey"
	"github.com/mcn/gateway-go/internal/store"
)

// ReversalRows is the saf_queue read ReversalLookup needs; *store.SafRepository satisfies it.
type ReversalRows interface {
	FindReversal(ctx context.Context, tranID int64) (store.SafRow, error)
}

// ReversalLookup reads a transaction's queued 0420 for its journey, decrypting the stored
// advice. The advice never holds DE 2, so nothing here can expose a PAN.
type ReversalLookup struct {
	rows   ReversalRows
	encKey []byte
}

// NewReversalLookup builds a ReversalLookup; encKey is the key saf_queue.payload_enc is sealed with.
func NewReversalLookup(rows ReversalRows, encKey []byte) *ReversalLookup {
	return &ReversalLookup{rows: rows, encKey: encKey}
}

// Reversal returns tranID's queued reversal, or nil when none was queued.
func (l *ReversalLookup) Reversal(ctx context.Context, tranID int64) (*journey.Reversal, error) {
	row, err := l.rows.FindReversal(ctx, tranID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find reversal tran_id=%d: %w", tranID, err)
	}
	adv, err := decodePayload(l.encKey, row.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode reversal tran_id=%d: %w", tranID, err)
	}
	return &journey.Reversal{Status: row.Status, Attempts: row.Attempts, QueuedAt: row.CreatedAt, AckedAt: row.AckedAt, Fields: adv.Fields}, nil
}
