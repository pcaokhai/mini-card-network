package rotation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRepository_createThenUpdateStepsThenComplete__MCN_504_AC1_AC3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	id, err := repo.Create(ctx, "ZPK")
	require.NoError(t, err)

	row, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "RUNNING", row.Status)
	require.Len(t, row.Steps, 4)
	for _, s := range row.Steps {
		require.Equal(t, "PENDING", s.Status)
	}

	require.NoError(t, repo.UpdateStep(ctx, id, StepGenerate, StatusDone))
	row, err = repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, StatusDone, stepStatus(row.Steps, StepGenerate))
	require.NotNil(t, stepCompletedAt(row.Steps, StepGenerate))

	require.NoError(t, repo.Complete(ctx, id, "AABBCC"))
	row, err = repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "COMPLETED", row.Status)
	require.Equal(t, "AABBCC", *row.NewKCV)

	require.NoError(t, repo.WriteAudit(ctx, "key_rotation.completed", map[string]any{"rotationId": id}))
}

func TestRepository_fail__MCN_504_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()

	id, err := repo.Create(ctx, "ZAK")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateStep(ctx, id, StepSend0800161, StatusFailed))
	require.NoError(t, repo.Fail(ctx, id))

	row, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "FAILED", row.Status)
	require.Equal(t, StatusFailed, stepStatus(row.Steps, StepSend0800161))
}

func stepStatus(steps []Step, name string) string {
	for _, s := range steps {
		if s.Name == name {
			return s.Status
		}
	}
	return ""
}

func stepCompletedAt(steps []Step, name string) *time.Time {
	for _, s := range steps {
		if s.Name == name {
			return s.CompletedAt
		}
	}
	return nil
}
