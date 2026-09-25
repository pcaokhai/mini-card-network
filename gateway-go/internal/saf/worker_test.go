package saf

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	testPAN       = "9704360000004417"
	testCardToken = "tok_normal"
)

type fakeMux struct {
	sentMTIs   []string
	sentFields []map[int]string
	response   map[int]string
	err        error
	stanCalls  int
	linkDown   bool
}

func (f *fakeMux) Send(_ context.Context, mti string, fields map[int]string) (map[int]string, error) {
	f.sentMTIs = append(f.sentMTIs, mti)
	f.sentFields = append(f.sentFields, fields)
	return f.response, f.err
}

func (f *fakeMux) NextSTAN() (string, bool) {
	if f.linkDown {
		return "", false
	}
	f.stanCalls++
	return "000777", true
}

type fakeCards map[string]string

func (f fakeCards) PAN(token string) (string, bool) {
	pan, ok := f[token]
	return pan, ok
}

// recordingHSM MACs deterministically and records what it was asked to MAC.
type recordingHSM struct {
	macInputs [][]byte
	calls     byte
}

func (r *recordingHSM) WrapUnderLMK(k []byte) ([]byte, error) { return k, nil }
func (r *recordingHSM) Unwrap(k []byte) ([]byte, error)       { return k, nil }
func (r *recordingHSM) ComputeKCV([]byte) (string, error)     { return "", nil }
func (r *recordingHSM) TranslatePIN(p, _, _ []byte) ([]byte, error) {
	return p, nil
}
func (r *recordingHSM) ComputeMAC(packed, _ []byte) ([]byte, error) {
	r.macInputs = append(r.macInputs, packed)
	r.calls++
	return []byte{1, 2, 3, 4, 5, 6, 7, r.calls}, nil
}

var _ hsm.Module = (*recordingHSM)(nil)

type fakeSaf struct {
	rows      []store.SafRow
	acked     []int64
	dead      []int64
	inFlights []int
	lastErrs  []string
	payloads  map[int64][]byte
}

func (f *fakeSaf) ClaimDue(context.Context, int) ([]store.SafRow, error) {
	due := f.rows
	f.rows = nil
	return due, nil
}
func (f *fakeSaf) MarkInFlight(_ context.Context, _ int64, attempts int, _ time.Time, lastError string) error {
	f.inFlights = append(f.inFlights, attempts)
	f.lastErrs = append(f.lastErrs, lastError)
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
func (f *fakeSaf) UpdatePayload(_ context.Context, id int64, payload []byte) error {
	if f.payloads == nil {
		f.payloads = map[int64][]byte{}
	}
	f.payloads[id] = payload
	return nil
}

func queuedRow(t *testing.T, id int64, attempts int) store.SafRow {
	t.Helper()
	adv, err := reversalAdvice(goldenOriginal(), "68")
	require.NoError(t, err)
	payload, err := encodePayload(nil, adv)
	require.NoError(t, err)
	return store.SafRow{ID: id, MTI: "0420", Attempts: attempts, MaxAttempts: 20, Payload: payload}
}

func newTestWorker(mux *fakeMux, safPort *fakeSaf, h *recordingHSM) *Worker {
	return NewWorker(mux, fakeCards{testCardToken: testPAN}, h, make([]byte, 16), safPort, nil,
		isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)
}

func TestWorker_deliversOneItemAndAcksOn0430__MCN_401_AC2(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 1, 0)}}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Equal(t, []string{"0420"}, mux.sentMTIs)
	require.Equal(t, []int64{1}, safPort.acked)
}

func TestWorker_repeatsAsX21OnSecondAttempt__MCN_401_AC2(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 2, 1)}}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Equal(t, []string{"0421"}, mux.sentMTIs)
}

func TestWorker_reschedulesOnSendFailureWithoutAcking__MCN_401_AC2(t *testing.T) {
	mux := &fakeMux{err: context.DeadlineExceeded}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 3, 0)}}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Empty(t, safPort.acked)
	require.Equal(t, []int{1}, safPort.inFlights)
}

func TestWorker_marksDeadAfterMaxAttempts__MCN_401_AC3(t *testing.T) {
	mux := &fakeMux{err: errors.New("issuer unreachable")}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 5, 19)}}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Equal(t, []int64{5}, safPort.dead)
}

func TestWorker_runStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := newTestWorker(&fakeMux{}, &fakeSaf{}, &recordingHSM{})

	require.NoError(t, w.Run(ctx))
}

func TestWorker_recordsWhyADeliveryFailed__MCN_401(t *testing.T) {
	mux := &fakeMux{err: errors.New("write request: broken pipe")}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 4, 0)}}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Equal(t, []string{"write request: broken pipe"}, safPort.lastErrs)
}

func TestWorker_assignsStanOnceAndRepeatsTheSameMessage__MCN_401(t *testing.T) {
	mux := &fakeMux{err: errors.New("no 0430 yet")}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 6, 0)}}
	w := newTestWorker(mux, safPort, &recordingHSM{})

	require.NoError(t, w.deliverOnce(context.Background()))
	first := mux.sentFields[0]
	require.Equal(t, "000777", first[11])
	require.Len(t, first[7], 10)

	// The retry reads back the payload the first attempt persisted.
	safPort.rows = []store.SafRow{{ID: 6, MTI: "0420", Attempts: 1, MaxAttempts: 20, Payload: safPort.payloads[6]}}
	require.NoError(t, w.deliverOnce(context.Background()))

	second := mux.sentFields[1]
	require.Equal(t, []string{"0420", "0421"}, mux.sentMTIs)
	require.Equal(t, 1, mux.stanCalls, "a repeat reuses the first attempt's STAN")
	require.Equal(t, first[11], second[11])
	require.Equal(t, first[7], second[7])
}

func TestWorker_macsEverySend__MCN_401(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	h := &recordingHSM{}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 7, 0)}}

	require.NoError(t, newTestWorker(mux, safPort, h).deliverOnce(context.Background()))

	sent := mux.sentFields[0]
	require.Equal(t, strings.ToUpper(hex.EncodeToString([]byte{1, 2, 3, 4, 5, 6, 7, 1})), sent[128])
	require.Len(t, h.macInputs, 1)
	require.True(t, strings.HasPrefix(string(h.macInputs[0]), "0420"), "the MTI is part of the MACed message")
	require.NotContains(t, string(h.macInputs[0]), sent[128], "DE 128 is excluded from its own MAC")
}

func TestWorker_neverPersistsThePAN__MCN_401(t *testing.T) {
	mux := &fakeMux{err: errors.New("no 0430 yet")}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 8, 0)}}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Equal(t, testPAN, mux.sentFields[0][2], "DE 2 is on the wire")
	require.False(t, bytes.Contains(safPort.payloads[8], []byte(testPAN)), "but never in saf_queue")
}

func TestWorker_noLiveLinkRetriesInsteadOfSendingWithoutAStan__MCN_401(t *testing.T) {
	mux := &fakeMux{linkDown: true}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 9, 0)}}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Empty(t, mux.sentMTIs)
	require.Equal(t, []int{1}, safPort.inFlights)
	require.NotEmpty(t, safPort.lastErrs[0])
}

// A 0430 that isn't "00" was not recorded (link not signed on is 91, a rejected frame 30), so the
// reversal is still owed: acknowledging it would mark REVERSED money the issuer never returned.
func TestWorker_onlyAn00AcknowledgementCompletesTheAdvice__MCN_401(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "30"}}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 10, 0)}}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Empty(t, safPort.acked)
	require.Equal(t, []int{1}, safPort.inFlights)
	require.Contains(t, safPort.lastErrs[0], "30")
}
