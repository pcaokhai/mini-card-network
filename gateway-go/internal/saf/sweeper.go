package saf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mcn/gateway-go/internal/store"
)

const (
	sweepBatch = 50

	tranTypeBalance    = "BALANCE"
	tranTypeCompletion = "COMPLETION"
	statusTimedOut     = "TIMED_OUT"
	mtiCompletion      = "0220"
	reasonTimeout      = "68" // docs/03 §7.3 DE 39 of the 0420: no response
)

// OrphanRows is the tran_log access the Sweeper needs; *store.TranLogRepository satisfies it.
type OrphanRows interface {
	FindOrphans(ctx context.Context, sentBefore time.Time, limit int) ([]store.TranLogRow, error)
	MarkTimedOut(ctx context.Context, id int64) (bool, error)
}

// OrphanQueuer queues the follow-up an unknown outcome owes; *ReversalQueuer satisfies it.
type OrphanQueuer interface {
	Queue(ctx context.Context, txn store.TranLogRow, reasonCode string) error
	QueueAdvice(ctx context.Context, tranID int64, mti string, sent map[int]string) error
}

// Sweeper follows up requests whose outcome the gateway never recorded (POS-G16): it crashed or
// failed between the send and the answer, or between the answer and queueing the reversal. A row
// older than age is an unknown outcome, so it gets what a timeout gets (root CLAUDE.md §6.4): a
// 0420, or for a completion a repeat of its 0220 (docs/03 §7.4); a balance inquiry owes nothing.
type Sweeper struct {
	rows     OrphanRows
	queuer   OrphanQueuer
	age      time.Duration
	interval time.Duration
	log      *slog.Logger
}

// NewSweeper builds a Sweeper that runs every interval over rows sent more than age ago. age
// must exceed the send timeout plus the time a request takes to record its outcome.
func NewSweeper(rows OrphanRows, queuer OrphanQueuer, age, interval time.Duration) *Sweeper {
	return &Sweeper{rows: rows, queuer: queuer, age: age, interval: interval, log: slog.Default()}
}

// SetLogger routes the sweeper's logs through the gateway's JSON logger.
func (s *Sweeper) SetLogger(l *slog.Logger) { s.log = l }

// Run sweeps every interval until ctx is cancelled. A failed sweep is retried on the next tick.
func (s *Sweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.SweepOnce(ctx); err != nil {
				s.log.ErrorContext(ctx, "sweep orphan transactions", "err", err) // retried next tick
			}
		}
	}
}

// SweepOnce follows up one batch of orphans; every orphan is attempted, and their errors joined.
func (s *Sweeper) SweepOnce(ctx context.Context) error {
	orphans, err := s.rows.FindOrphans(ctx, time.Now().Add(-s.age), sweepBatch)
	if err != nil {
		return fmt.Errorf("find orphan transactions: %w", err)
	}
	var errs []error
	for _, row := range orphans {
		if err := s.followUp(ctx, row); err != nil {
			errs = append(errs, fmt.Errorf("follow up %s: %w", row.RRN, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Sweeper) followUp(ctx context.Context, row store.TranLogRow) error {
	if row.Status != statusTimedOut {
		moved, err := s.rows.MarkTimedOut(ctx, row.ID)
		if err != nil || !moved {
			return err // not moved: its request recorded an outcome after all
		}
		row.Status = statusTimedOut
	}
	switch row.Type {
	case tranTypeBalance:
		return nil
	case tranTypeCompletion:
		return s.queuer.QueueAdvice(ctx, row.ID, mtiCompletion, completionAdvice(row))
	}
	return s.queuer.Queue(ctx, row, reasonTimeout)
}

// completionAdvice rebuilds the 0220 a completion sent (advtxn.CreateCompletion) from its row: the
// same STAN and DE 7, so every repeat is the same message the issuer dedupes.
func completionAdvice(row store.TranLogRow) map[int]string {
	return map[int]string{
		3:  row.ProcessingCode,
		4:  fmt.Sprintf("%012d", row.Amount),
		7:  row.SentAt.UTC().Format("0102150405"),
		11: row.NetworkSTAN,
		32: acquirerID,
		37: row.OriginalRRN,
		41: row.TerminalID,
		42: row.MerchantID,
		49: row.Currency,
	}
}
