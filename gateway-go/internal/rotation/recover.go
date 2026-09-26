package rotation

import (
	"context"
	"fmt"
)

// recoverInterrupted settles the rotations a crash left RUNNING, by how far each got:
//   - ACTIVATE done: the new key is in use, so the rotation is completed (review N1);
//   - a key was generated but not activated: the 0800 may have reached the issuer, so the
//     rotation fails "outcome unknown" and the key stays PENDING for the MAC fallback (S2);
//   - no key yet: nothing left the gateway; the rotation just fails.
func (r *Runner) recoverInterrupted(ctx context.Context) error {
	running, err := r.repo.ListRunning(ctx)
	if err != nil {
		return err
	}
	for _, row := range running {
		if err := r.recoverOne(ctx, row); err != nil {
			return fmt.Errorf("rotation %d: %w", row.ID, err)
		}
	}
	return nil
}

func (r *Runner) recoverOne(ctx context.Context, row Row) error {
	if stepStatusOf(row.Steps, StepActivate) == StatusDone && row.NewKeyID != nil {
		key, err := r.keyStore.Get(ctx, *row.NewKeyID)
		if err != nil {
			return err
		}
		return r.repo.Complete(ctx, row.ID, key.KCV)
	}
	failed := firstPending(row.Steps)
	cause := "interrupted before the key was generated"
	if row.NewKeyID != nil {
		cause = "interrupted: outcome unknown, the new key stays PENDING"
	}
	_, _ = r.failStep(ctx, row.ID, failed, 0, fmt.Errorf("%s", cause))
	return nil
}

func stepStatusOf(steps []Step, name string) string {
	for _, s := range steps {
		if s.Name == name {
			return s.Status
		}
	}
	return ""
}

// firstPending is the step the rotation was on; ACTIVATE if every step is marked.
func firstPending(steps []Step) string {
	for _, s := range steps {
		if s.Status == StatusPending {
			return s.Name
		}
	}
	return StepActivate
}
