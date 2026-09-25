package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type recordingTerminals struct{ got []store.FixtureTerminal }

func (r *recordingTerminals) UpsertFromFixture(_ context.Context, t []store.FixtureTerminal) error {
	r.got = t
	return nil
}

type recordingBackdater struct{ at map[string]time.Time }

func (r *recordingBackdater) Backdate(_ context.Context, rrn string, at time.Time) error {
	r.at[rrn] = at
	return nil
}

// fakeStack answers like a signed-on gateway whose issuer decides by the fixture card states, and
// like the issuer Card Admin API.
type fakeStack struct {
	mu        sync.Mutex
	nextRRN   int
	byRRN     map[string]string // rrn -> status
	purchases []map[string]any
	ifMatch   string
	limits    map[string]any
	onCancel  func()
	declineRC string // when set, every purchase is declined with it
}

func (f *fakeStack) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/cards/crd_limit00005", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("PUT /v1/cards/crd_limit00005/limits", func(w http.ResponseWriter, r *http.Request) {
		f.ifMatch = r.Header.Get("If-Match")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&f.limits))
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("POST /v1/transactions/purchases", func(w http.ResponseWriter, r *http.Request) {
		require.NotEmpty(t, r.Header.Get("Idempotency-Key"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		f.mu.Lock()
		f.purchases = append(f.purchases, body)
		f.nextRRN++
		rrn := fmt.Sprintf("626514%06d", f.nextRRN)
		status, rc := f.decide(body)
		f.byRRN[rrn] = status
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"rrn": rrn, "status": status, "responseCode": rc})
	})
	mux.HandleFunc("POST /v1/transactions/{rrn}/cancellations", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.byRRN[r.PathValue("rrn")] = "REVERSED" // the SAF worker's ACK, collapsed
		f.mu.Unlock()
		if f.onCancel != nil {
			f.onCancel()
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("GET /v1/transactions/{rrn}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		status := f.byRRN[r.PathValue("rrn")]
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"rrn": r.PathValue("rrn"), "status": status})
	})
	return mux
}

func (f *fakeStack) decide(body map[string]any) (status, rc string) {
	if f.declineRC != "" {
		return "DECLINED", f.declineRC
	}
	amount := int64(body["amount"].(map[string]any)["amount"].(float64))
	switch body["cardToken"] {
	case "tok_low":
		return "DECLINED", "51"
	case "tok_blocked":
		return "DECLINED", "62"
	case "tok_expired":
		return "DECLINED", "54"
	case "tok_limit":
		if amount > LimitPerTransaction {
			return "DECLINED", "61"
		}
	}
	return "APPROVED", "00"
}

func newTestRunner(t *testing.T, stack *fakeStack, now *time.Time) (*Runner, *recordingTerminals, *recordingBackdater) {
	t.Helper()
	fixture, err := LoadFixture(fixturePath)
	require.NoError(t, err)
	stack.byRRN = map[string]string{}
	srv := httptest.NewServer(stack.handler(t))
	t.Cleanup(srv.Close)
	terminals := &recordingTerminals{}
	backdater := &recordingBackdater{at: map[string]time.Time{}}
	return &Runner{
		Fixture: fixture, GatewayURL: srv.URL, IssuerAdminURL: srv.URL, HTTP: srv.Client(),
		Terminals: terminals, TranLog: backdater, Rand: rand.New(rand.NewSource(11)), //nolint:gosec // deterministic test data
		Now: func() time.Time { return *now }, SettleTimeout: time.Second, PollInterval: time.Millisecond,
		Logf: t.Logf,
	}, terminals, backdater
}

func TestRunner_seedsEveryPlannedOutcomeAndBackdatesByTheElapsedTime__MCN_002(t *testing.T) {
	now := planNow
	stack := &fakeStack{}
	stack.onCancel = func() { now = planNow.Add(90 * time.Second) }
	runner, terminals, backdater := newTestRunner(t, stack, &now)
	plan := Build(planNow, rand.New(rand.NewSource(11))) //nolint:gosec // deterministic test data

	sum, err := runner.Run(context.Background())

	require.NoError(t, err)
	require.Empty(t, sum.Mismatches)
	require.Len(t, terminals.got, 7)
	require.Equal(t, `"v1"`, stack.ifMatch, "limits are updated with the card's ETag")
	require.EqualValues(t, LimitPerTransaction, stack.limits["perTransactionAmount"].(map[string]any)["amount"])
	require.Len(t, stack.purchases, len(plan))
	require.Equal(t, 2, sum.Reversed)
	for _, rc := range []string{"51", "61", "62", "54"} {
		require.Positive(t, sum.Outcomes[rc])
	}
	require.Len(t, backdater.at, len(plan))
	for i, txn := range plan {
		rrn := fmt.Sprintf("626514%06d", i+1)
		require.Equal(t, txn.At.Add(90*time.Second), backdater.at[rrn], "rrn %s", rrn)
	}
}

func TestRunner_stopsWhenTheStackCannotAuthorise__MCN_002(t *testing.T) {
	now := planNow
	stack := &fakeStack{declineRC: "96"}
	runner, _, backdater := newTestRunner(t, stack, &now)

	_, err := runner.Run(context.Background())

	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "signed on"), err.Error())
	require.Len(t, stack.purchases, earlyAbortAfter)
	require.Empty(t, backdater.at, "nothing is backdated after an aborted run")
}
