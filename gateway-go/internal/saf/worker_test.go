package saf

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/store"
)

type fakeMux struct {
	sentMTIs []string
	response map[int]string
	err      error
}

func (f *fakeMux) Send(_ context.Context, mti string, _ map[int]string) (map[int]string, error) {
	f.sentMTIs = append(f.sentMTIs, mti)
	return f.response, f.err
}

type fakeSaf struct {
	rows      []store.SafRow
	acked     []int64
	dead      []int64
	inFlights []int
}

func (f *fakeSaf) ClaimDue(context.Context, int) ([]store.SafRow, error) {
	due := f.rows
	f.rows = nil
	return due, nil
}
func (f *fakeSaf) MarkInFlight(_ context.Context, _ int64, attempts int, _ time.Time) error {
	f.inFlights = append(f.inFlights, attempts)
	return nil
}
func (f *fakeSaf) MarkAcked(_ context.Context, id int64) error {
	f.acked = append(f.acked, id)
	return nil
}
func (f *fakeSaf) MarkDead(_ context.Context, id int64, _ string) error {
	f.dead = append(f.dead, id)
	return nil
}

func TestWorker_deliversOneItemAndAcksOn0430__MCN_401_AC2(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	safPort := &fakeSaf{rows: []store.SafRow{{ID: 1, MTI: "0420", Attempts: 0, MaxAttempts: 20}}}
	w := NewWorker(mux, safPort, nil, isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)

	require.NoError(t, w.deliverOnce(context.Background()))

	require.Equal(t, []string{"0420"}, mux.sentMTIs)
	require.Equal(t, []int64{1}, safPort.acked)
}

func TestWorker_repeatsAsX21OnSecondAttempt__MCN_401_AC2(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	safPort := &fakeSaf{rows: []store.SafRow{{ID: 2, MTI: "0420", Attempts: 1, MaxAttempts: 20}}}
	w := NewWorker(mux, safPort, nil, isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)

	require.NoError(t, w.deliverOnce(context.Background()))

	require.Equal(t, []string{"0421"}, mux.sentMTIs)
}

func TestWorker_reschedulesOnSendFailureWithoutAcking__MCN_401_AC2(t *testing.T) {
	mux := &fakeMux{err: context.DeadlineExceeded}
	safPort := &fakeSaf{rows: []store.SafRow{{ID: 3, MTI: "0420", Attempts: 0, MaxAttempts: 20}}}
	w := NewWorker(mux, safPort, nil, isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)

	require.NoError(t, w.deliverOnce(context.Background()))

	require.Empty(t, safPort.acked)
	require.Equal(t, []int{1}, safPort.inFlights)
}

func TestWorker_marksDeadAfterMaxAttempts__MCN_401_AC3(t *testing.T) {
	mux := &fakeMux{err: context.DeadlineExceeded}
	safPort := &fakeSaf{rows: []store.SafRow{{ID: 4, MTI: "0420", Attempts: 20, MaxAttempts: 20}}}
	w := NewWorker(mux, safPort, nil, isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)

	require.NoError(t, w.deliverOnce(context.Background()))

	require.Equal(t, []int64{4}, safPort.dead)
	require.Empty(t, safPort.acked)
}

func TestWorker_runStopsOnContextCancel(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	safPort := &fakeSaf{}
	w := NewWorker(mux, safPort, nil, isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	require.NoError(t, w.Run(ctx))
}
