package rotation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

const (
	typeZPK = "ZPK"
	typeZAK = "ZAK"
)

var testZMK = []byte("zmk-test-key-32-bytes-----------")

// blockingMux holds every Send until release is closed or the context ends.
type blockingMux struct {
	release chan struct{}
	started chan struct{}
	once    sync.Once
}

func newBlockingMux() *blockingMux {
	return &blockingMux{release: make(chan struct{}), started: make(chan struct{})}
}

func (m *blockingMux) NextSTAN() (string, bool) { return "000001", true }

func (m *blockingMux) Send(ctx context.Context, _ string, _ map[int]string) (map[int]string, error) {
	m.once.Do(func() { close(m.started) })
	select {
	case <-m.release:
		return map[int]string{39: "00"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func serveRunner(t *testing.T, runner *Runner) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Serve(ctx) }()
	require.Eventually(t, runner.serving, time.Second, 5*time.Millisecond)
	stopped := false
	stop = func() error {
		if stopped {
			return nil
		}
		stopped = true
		cancel()
		return <-done
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

func waitStatus(t *testing.T, repo *Repository, id int64, status string) Row {
	t.Helper()
	var row Row
	require.Eventually(t, func() bool {
		var err error
		row, err = repo.Get(context.Background(), id)
		return err == nil && row.Status == status
	}, 3*time.Second, 10*time.Millisecond)
	return row
}

func TestRunner_startReturnsRunningAndCompletesInTheBackground__SEC_G2(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	mux := newBlockingMux()
	runner := NewRunner(repo, store.NewKeyStoreRepository(pool), fakeHSM{}, mux, testZMK)
	serveRunner(t, runner)

	row, err := runner.Start(context.Background(), typeZPK, "")
	require.NoError(t, err)
	require.Equal(t, "RUNNING", row.Status)
	require.NotZero(t, row.ID)

	<-mux.started
	close(mux.release)
	final := waitStatus(t, repo, row.ID, "COMPLETED")
	require.Equal(t, StatusDone, stepStatus(final.Steps, StepActivate))
}

func TestRunner_shutdownStopsARunningRotationAsFailed__SEC_G2(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	keyStore := store.NewKeyStoreRepository(pool)
	mux := newBlockingMux()
	runner := NewRunner(repo, keyStore, fakeHSM{}, mux, testZMK)
	stop := serveRunner(t, runner)

	row, err := runner.Start(context.Background(), typeZPK, "")
	require.NoError(t, err)
	<-mux.started

	require.NoError(t, stop(), "Serve returns once the rotation goroutine has stopped")
	final, err := repo.Get(context.Background(), row.ID)
	require.NoError(t, err)
	require.Equal(t, "FAILED", final.Status)
	require.Equal(t, StatusFailed, stepStatus(final.Steps, StepSend0800161))

	require.Len(t, pendingKeys(t, keyStore), 1, "a 0800 cut off by shutdown has an unknown outcome: the key stays PENDING")

	_, err = runner.Start(context.Background(), typeZPK, "")
	require.ErrorIs(t, err, ErrNotServing)
}

func TestRunner_rejectsASecondRotationWhileOneRuns__SEC_G2(t *testing.T) {
	pool := newTestPool(t)
	mux := newBlockingMux()
	runner := NewRunner(NewRepository(pool), store.NewKeyStoreRepository(pool), fakeHSM{}, mux, testZMK)
	serveRunner(t, runner)

	_, err := runner.Start(context.Background(), typeZPK, "")
	require.NoError(t, err)
	_, err = runner.Start(context.Background(), typeZAK, "")
	require.ErrorIs(t, err, ErrRotationInProgress)
	close(mux.release)
}

func TestRunner_notifiesActivationSoTheNewKeyIsUsed__SEC_G10(t *testing.T) {
	pool := newTestPool(t)
	var mu sync.Mutex
	var activated []string
	runner := NewRunner(NewRepository(pool), store.NewKeyStoreRepository(pool), fakeHSM{}, &fakeMux{}, testZMK,
		WithActivationHook(func(_ context.Context, keyType string) {
			mu.Lock()
			defer mu.Unlock()
			activated = append(activated, keyType)
		}))

	_, err := runner.Run(context.Background(), typeZAK, "")
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{typeZAK}, activated)
}

func TestRepository_getUnknownIsErrNotFound__SEC_G6(t *testing.T) {
	_, err := NewRepository(newTestPool(t)).Get(context.Background(), 999999)
	require.ErrorIs(t, err, ErrNotFound)
}
