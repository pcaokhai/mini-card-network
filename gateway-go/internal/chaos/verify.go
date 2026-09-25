package chaos

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/purchase"
)

// rcMacFailure is the RC a purchase gets when the issuer's response MAC failed verification; the
// gateway queues a reversal for it like for a timeout (MCN-502-AC3).
const rcMacFailure = "96"

// runError ends a run FAILED with RUN_ERROR: detail is the operator-readable failureDetail the API
// returns, cause the raw error, which only goes to the log (CHA-G3, review N3).
type runError struct {
	detail string
	cause  error
}

func (e *runError) Error() string { return e.detail }
func (e *runError) Unwrap() error { return e.cause }

// runErr wraps cause with detail, unless cause already carries a more specific detail.
func runErr(detail string, cause error) error {
	var inner *runError
	if errors.As(cause, &inner) {
		return cause
	}
	return &runError{detail: detail, cause: cause}
}

// reversalQueued reports whether the gateway queued a reversal for txn: after a timeout, or after
// a response whose MAC failed verification (declined with RC 96).
func reversalQueued(txn purchase.Transaction) bool {
	switch {
	case txn.Status == "TIMED_OUT", txn.Status == "REVERSAL_PENDING":
		return true
	case txn.Status == "DECLINED" && txn.ResponseCode == rcMacFailure:
		return true
	}
	return false
}

func (r *Runner) countReversed(ctx context.Context, runID string, rrns []string) error {
	for _, rrn := range rrns {
		row, err := r.tranLog.Get(ctx, rrn)
		if err != nil {
			return fmt.Errorf("read tran_log rrn=%s: %w", rrn, err)
		}
		if row.Status == statusReversed {
			r.update(runID, func(rn *ChaosRun) { rn.Reversed++ })
		}
	}
	return nil
}

// verify compares the issuer's balance movement with what the run's outcomes say it should be.
// Only approvals move money: a timed-out purchase is either never booked or booked and reversed,
// so it nets to 0 once SAF drains - and if its reversal never landed, that is exactly the
// mismatch this check exists to catch. The check assumes nothing else moves the seed cards'
// balances during the run.
func (r *Runner) verify(runID string, opening, closing, approvedTotal int64) {
	expected := -approvedTotal
	discrepancy := (closing - opening) - expected
	r.update(runID, func(rn *ChaosRun) {
		rn.ClosingBalanceTotal = closing
		rn.LedgerDiscrepancy = discrepancy
		rn.Status = statusPassed
		if discrepancy != 0 {
			rn.Status = statusFailed
			rn.FailureKind = ptr(failureLedgerMismatch)
			rn.FailureDetail = ptr(fmt.Sprintf("issuer balances moved by %d, run outcomes expect %d "+
				"(the check assumes no other traffic on the seed cards during the run)", closing-opening, expected))
		}
	})
}

// failRun ends the run on an infrastructure error: not a money verdict (CHA-G3). The API gets a
// generic detail; the raw error, PAN-masked, is logged against the run's trace id.
func (r *Runner) failRun(ctx context.Context, runID string, err error) {
	detail := "the run could not complete"
	var re *runError
	if errors.As(err, &re) {
		detail = re.detail
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		detail = fmt.Sprintf("the run exceeded its %s time limit", r.maxRunDuration)
	case errors.Is(ctx.Err(), context.Canceled):
		detail = "the gateway shut down during the run"
	}
	cause := err.Error()
	if re != nil && re.cause != nil {
		cause = re.cause.Error()
	}
	r.log.ErrorContext(ctx, "chaos run failed", "run_id", runID, "detail", detail, "error", obs.MaskPAN(cause))
	r.update(runID, func(rn *ChaosRun) {
		rn.Status = statusFailed
		rn.FailureKind = ptr(failureRunError)
		rn.FailureDetail = ptr(detail)
	})
}

// balanceTotal sums the seed cards' ledger balances at the issuer. Amounts in different
// currencies can't be summed, so a mixed set is an error rather than a meaningless total.
func (r *Runner) balanceTotal(ctx context.Context) (int64, error) {
	var total int64
	var cur string
	for _, c := range r.seedCards {
		ref, ok := fixtureCardRefs[c.CardToken]
		if !ok {
			return 0, fmt.Errorf("no cardRef for seed card %s", c.CardToken)
		}
		balance, cardCurrency, err := r.balances.LedgerBalance(ctx, ref)
		if err != nil {
			return 0, fmt.Errorf("card %s: %w", ref, err)
		}
		if cur != "" && cardCurrency != cur {
			return 0, runErr("the seed cards hold different currencies, so their balances can't be summed",
				fmt.Errorf("card %s is in %s, earlier cards in %s", ref, cardCurrency, cur))
		}
		cur = cardCurrency
		total += balance
	}
	return total, nil
}

// waitForSafDrain waits up to safDrainTimeout for the SAF queue to empty. Pending reversals move
// money, so an undrained queue makes any balance comparison meaningless: it ends the run
// RUN_ERROR, never a ledger verdict (review S1/S2).
func (r *Runner) waitForSafDrain(ctx context.Context, phase string) error {
	deadline := time.Now().Add(r.safDrainTimeout)
	poll := min(safDrainPollInterval, r.safDrainTimeout/10)
	for {
		_, depth, err := r.saf.ListPending(ctx)
		if err == nil && depth == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return runErr(fmt.Sprintf("SAF not drained %s (the queue could not be read)", phase), err)
			}
			return runErr(fmt.Sprintf("SAF not drained %s (%d pending)", phase, depth), nil)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}
