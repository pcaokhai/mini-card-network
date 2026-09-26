package rotation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

func pendingKeys(t *testing.T, keyStore *store.KeyStoreRepository) []store.KeyRow {
	t.Helper()
	current, err := keyStore.ListCurrent(context.Background())
	require.NoError(t, err)
	var pending []store.KeyRow
	for _, k := range current {
		if k.Status == StatusPending {
			pending = append(pending, k)
		}
	}
	return pending
}

func TestRunner_aLost0810IsRetriedWithTheSameKey__SEC_S2(t *testing.T) {
	pool := newTestPool(t)
	mux := &fakeMux{errs: []error{context.DeadlineExceeded}}
	runner := NewRunner(NewRepository(pool), store.NewKeyStoreRepository(pool), fakeHSM{}, mux, testZMK, WithSendAttempts(3))

	row, err := runner.Run(context.Background(), typeZPK, "")

	require.NoError(t, err)
	require.Equal(t, "COMPLETED", row.Status)
	require.Len(t, mux.de48s, 2)
	require.Equal(t, mux.de48s[0], mux.de48s[1], "a retry resends the same key, never a new one")
}

func TestRunner_anUnknownOutcomeFailsButKeepsThePendingKey__SEC_S2(t *testing.T) {
	pool := newTestPool(t)
	keyStore := store.NewKeyStoreRepository(pool)
	mux := &fakeMux{sendErr: context.DeadlineExceeded}
	runner := NewRunner(NewRepository(pool), keyStore, fakeHSM{}, mux, testZMK, WithSendAttempts(3))

	row, err := runner.Run(context.Background(), typeZPK, "")

	require.Error(t, err)
	require.Equal(t, "FAILED", row.Status)
	require.Equal(t, StatusFailed, stepStatus(row.Steps, StepSend0800161))
	require.Len(t, mux.de48s, 3)
	require.Len(t, pendingKeys(t, keyStore), 1, "the issuer may have activated it: never retire an unknown outcome")
}

func TestRunner_anExplicitDeclineRetiresTheNewKey__SEC_S2(t *testing.T) {
	pool := newTestPool(t)
	keyStore := store.NewKeyStoreRepository(pool)
	runner := NewRunner(NewRepository(pool), keyStore, fakeHSM{}, &fakeMux{rc: "96"}, testZMK)

	row, err := runner.Run(context.Background(), typeZPK, "")

	require.Error(t, err)
	require.Equal(t, "FAILED", row.Status)
	require.Empty(t, pendingKeys(t, keyStore))
}

// capturingHSM remembers the clear keys it was given, to check they are zeroed afterwards.
type capturingHSM struct {
	fakeHSM
	clearKeys *[][]byte
}

func (c capturingHSM) WrapUnderLMK(k []byte) ([]byte, error) {
	*c.clearKeys = append(*c.clearKeys, k)
	return c.fakeHSM.WrapUnderLMK(k)
}

func TestRunner_zeroesTheClearKeyOnceWrapped__SEC_N5(t *testing.T) {
	pool := newTestPool(t)
	var seen [][]byte
	runner := NewRunner(NewRepository(pool), store.NewKeyStoreRepository(pool), capturingHSM{clearKeys: &seen}, &fakeMux{}, testZMK)

	_, err := runner.Run(context.Background(), typeZPK, "")

	require.NoError(t, err)
	require.Len(t, seen, 1)
	require.Equal(t, make([]byte, clearKeyLenBytes), seen[0])
}

func TestRunner_serveResolvesInterruptedRotationsByStep__SEC_N1_S2(t *testing.T) {
	pool := newTestPool(t)
	repo := NewRepository(pool)
	keyStore := store.NewKeyStoreRepository(pool)
	ctx := context.Background()

	activated, err := repo.Create(ctx, typeZPK) // crashed after ACTIVATE, before COMPLETED
	require.NoError(t, err)
	activatedKey, err := keyStore.Insert(ctx, store.KeyRow{KeyType: typeZPK, KeyUnderLMKHex: "aa", KCV: "AAAAAA"})
	require.NoError(t, err)
	require.NoError(t, repo.SetNewKey(ctx, activated, activatedKey))
	require.NoError(t, keyStore.Activate(ctx, activatedKey))
	for _, step := range []string{StepGenerate, StepSend0800161, StepPartnerConfirm, StepActivate} {
		require.NoError(t, repo.UpdateStep(ctx, activated, step, StatusDone))
	}

	sent, err := repo.Create(ctx, typeZAK) // crashed after the key was generated
	require.NoError(t, err)
	sentKey, err := keyStore.Insert(ctx, store.KeyRow{KeyType: typeZAK, KeyUnderLMKHex: "bb", KCV: "BBBBBB"})
	require.NoError(t, err)
	require.NoError(t, repo.SetNewKey(ctx, sent, sentKey))
	require.NoError(t, repo.UpdateStep(ctx, sent, StepGenerate, StatusDone))

	early, err := repo.Create(ctx, typeZAK) // crashed before GENERATE
	require.NoError(t, err)

	serveRunner(t, NewRunner(repo, keyStore, fakeHSM{}, &fakeMux{}, testZMK))

	done := waitStatus(t, repo, activated, "COMPLETED")
	require.Equal(t, "AAAAAA", *done.NewKCV)
	require.Equal(t, "FAILED", waitStatus(t, repo, sent, "FAILED").Status)
	require.Equal(t, "FAILED", waitStatus(t, repo, early, "FAILED").Status)
	require.Len(t, pendingKeys(t, keyStore), 1, "the generated key's outcome is unknown, so it stays PENDING")
}
