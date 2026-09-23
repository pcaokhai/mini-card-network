package rotation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeHSM struct{}

func (fakeHSM) WrapUnderLMK(k []byte) ([]byte, error)       { return append([]byte("wrapped:"), k...), nil }
func (fakeHSM) Unwrap(k []byte) ([]byte, error)             { return k, nil }
func (fakeHSM) ComputeKCV([]byte) (string, error)           { return "AABBCC", nil }
func (fakeHSM) ComputeMAC([]byte, []byte) ([]byte, error)   { return make([]byte, 8), nil }
func (fakeHSM) TranslatePIN(p, _, _ []byte) ([]byte, error) { return p, nil }

type fakeMux struct {
	confirmed bool
	sendErr   error
	rc        string
}

func (f *fakeMux) NextSTAN() (string, bool) { return "000001", true }

func (f *fakeMux) Send(_ context.Context, _ string, _ map[int]string) (map[int]string, error) {
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	f.confirmed = true
	rc := f.rc
	if rc == "" {
		rc = "00"
	}
	return map[int]string{39: rc}, nil
}

func TestRunner_run_completesAllFourStepsAndActivates__MCN_504_AC1_AC3(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	keyStore := store.NewKeyStoreRepository(pool)
	mux := &fakeMux{}
	runner := NewRunner(repo, keyStore, fakeHSM{}, mux, []byte("zmk-test-key-32-bytes-----------"))

	row, err := runner.Run(context.Background(), "ZPK", "gw-link-01")

	require.NoError(t, err)
	require.Equal(t, "COMPLETED", row.Status)
	require.Equal(t, "AABBCC", *row.NewKCV)
	require.True(t, mux.confirmed)
	for _, s := range row.Steps {
		require.Equal(t, StatusDone, s.Status)
	}

	keys, err := keyStore.List(context.Background())
	require.NoError(t, err)
	require.Contains(t, activeStatusesFor(keys, "ZPK"), "ACTIVE")
}

func TestRunner_run_marksFailedWhenPartnerConfirmErrors__MCN_504_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	keyStore := store.NewKeyStoreRepository(pool)
	mux := &fakeMux{rc: "96"}
	runner := NewRunner(repo, keyStore, fakeHSM{}, mux, []byte("zmk-test-key-32-bytes-----------"))

	row, err := runner.Run(context.Background(), "ZAK", "gw-link-01")

	require.Error(t, err)
	require.Equal(t, "FAILED", row.Status)
	require.Equal(t, StatusDone, stepStatus(row.Steps, StepGenerate))
	require.Equal(t, StatusDone, stepStatus(row.Steps, StepSend0800161))
	require.Equal(t, StatusFailed, stepStatus(row.Steps, StepPartnerConfirm))
}

func TestRunner_run_marksFailedWhenSendErrors__MCN_504_AC1(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	keyStore := store.NewKeyStoreRepository(pool)
	mux := &fakeMux{sendErr: errors.New("link down")}
	runner := NewRunner(repo, keyStore, fakeHSM{}, mux, []byte("zmk-test-key-32-bytes-----------"))

	row, err := runner.Run(context.Background(), "ZAK", "gw-link-01")

	require.Error(t, err)
	require.Equal(t, "FAILED", row.Status)
	require.Equal(t, StatusDone, stepStatus(row.Steps, StepGenerate))
	require.Equal(t, StatusFailed, stepStatus(row.Steps, StepSend0800161))
}

func activeStatusesFor(keys []store.KeyRow, keyType string) []string {
	var statuses []string
	for _, k := range keys {
		if k.KeyType == keyType {
			statuses = append(statuses, k.Status)
		}
	}
	return statuses
}
