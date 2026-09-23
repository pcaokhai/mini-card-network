// Package saf owns the store-and-forward queue's delivery worker and the reversal path that
// feeds it (docs/03-iso8583-interface-spec.md §7.3, §9).
package saf

import (
	"context"
	"fmt"
	"time"

	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/store"
)

const claimBatchSize = 50

// MuxSender sends a request on the acquirer's live issuer connection. *isonet.Supervisor
// satisfies it (same shape purchase.Service's MuxSender already uses).
type MuxSender interface {
	Send(ctx context.Context, mti string, fields map[int]string) (map[int]string, error)
}

// Port is the saf_queue access Worker needs. *store.SafRepository satisfies it.
type Port interface {
	ClaimDue(ctx context.Context, limit int) ([]store.SafRow, error)
	MarkInFlight(ctx context.Context, id int64, attempts int, nextRetryAt time.Time) error
	MarkAcked(ctx context.Context, id int64) error
	MarkDead(ctx context.Context, id int64, lastError string) error
}

// Worker polls saf_queue and delivers due advices, repeating as x21 after the first attempt,
// full-jitter backing off between attempts, and dead-lettering after max_attempts.
type Worker struct {
	mux          MuxSender
	saf          Port
	encKey       []byte
	backoff      isonet.Backoff
	pollInterval time.Duration
}

// NewWorker builds a Worker. encKey decrypts saf_queue.payload_enc (nil is fine when every
// claimed row carries an empty payload, e.g. in tests).
func NewWorker(mux MuxSender, saf Port, encKey []byte, backoff isonet.Backoff, pollInterval time.Duration) *Worker {
	return &Worker{mux: mux, saf: saf, encKey: encKey, backoff: backoff, pollInterval: pollInterval}
}

// Run polls at pollInterval until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.deliverOnce(ctx); err != nil {
				return err
			}
		}
	}
}

// deliverOnce claims every due row and attempts one delivery each.
func (w *Worker) deliverOnce(ctx context.Context) error {
	due, err := w.saf.ClaimDue(ctx, claimBatchSize)
	if err != nil {
		return fmt.Errorf("claim due saf rows: %w", err)
	}
	for _, row := range due {
		if err := w.deliverRow(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

// deliverRow sends one advice. attempts==0 sends the original MTI; every repeat is x21 (docs/03
// §7.3: attempts > 0 means a repeat, never a second original send). Any response ACKs - advices
// are never declined by the issuer, only lost in transit.
func (w *Worker) deliverRow(ctx context.Context, row store.SafRow) error {
	mti := row.MTI
	if row.Attempts > 0 {
		mti = mti[:3] + "1"
	}

	fields, decodeErr := decodePayload(w.encKey, row.Payload)
	if decodeErr != nil {
		return fmt.Errorf("decode saf payload id=%d: %w", row.ID, decodeErr)
	}

	_, err := w.mux.Send(ctx, mti, fields)
	if err == nil {
		return w.saf.MarkAcked(ctx, row.ID)
	}

	attempts := row.Attempts + 1
	if attempts >= row.MaxAttempts {
		obs.SafDeadTotal.Inc()
		return w.saf.MarkDead(ctx, row.ID, err.Error())
	}
	nextRetryAt := time.Now().Add(w.backoff.Delay(attempts))
	return w.saf.MarkInFlight(ctx, row.ID, attempts, nextRetryAt)
}
