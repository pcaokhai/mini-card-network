package saf

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	sentDE7       = "0925101500" // the DE 7 a queued advice was first sent with
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
	lost       bool // the 0430 never arrives: Send waits for its context
}

func (f *fakeMux) Send(ctx context.Context, mti string, fields map[int]string) (map[int]string, error) {
	f.sentMTIs = append(f.sentMTIs, mti)
	f.sentFields = append(f.sentFields, fields)
	if f.lost {
		<-ctx.Done()
		return nil, ctx.Err()
	}
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
	ackErr    error
	lease     time.Duration
}

func (f *fakeSaf) ClaimDue(_ context.Context, _ int, lease time.Duration) ([]store.SafRow, error) {
	f.lease = lease
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
	if f.ackErr != nil {
		return f.ackErr
	}
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

// sentRow is a queued advice whose STAN and DE 7 an earlier claim already assigned, i.e. one that
// may already be on the issuer's side.
func sentRow(t *testing.T, id int64, attempts int) store.SafRow {
	t.Helper()
	adv, err := reversalAdvice(goldenOriginal(), "68")
	require.NoError(t, err)
	adv.Fields[11], adv.Fields[7] = "000777", sentDE7
	payload, err := encodePayload(nil, adv)
	require.NoError(t, err)
	return store.SafRow{ID: id, MTI: "0420", Attempts: attempts, MaxAttempts: 20, Payload: payload}
}

func newTestWorker(mux *fakeMux, safPort *fakeSaf, h *recordingHSM) *Worker {
	return NewWorker(mux, fakeCards{testCardToken: testPAN}, h, hsm.StaticZAK(make([]byte, 16)), acceptAllMACs{}, safPort, nil,
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
	safPort := &fakeSaf{rows: []store.SafRow{sentRow(t, 2, 1)}}

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
	require.Equal(t, []int{0}, safPort.inFlights, "nothing was sent, so it is not an attempt")
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

// A lost 0430 must not hold the worker forever: the send gives up and the advice is retried.
func TestWorker_aLost0430TimesOutAndIsRetried__MCN_401(t *testing.T) {
	mux := &fakeMux{lost: true}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 11, 0)}}
	w := newTestWorker(mux, safPort, &recordingHSM{})
	w.sendTimeout = 10 * time.Millisecond

	require.NoError(t, w.deliverOnce(context.Background()))

	require.Equal(t, []int{1}, safPort.inFlights)
	require.Contains(t, safPort.lastErrs[0], "deadline")
}

// A failed store write is one row's problem: the rest of the batch is still delivered and the
// worker (and with it the gateway) keeps running.
func TestWorker_aStoreErrorDoesNotStopTheBatch__MCN_401(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 12, 0), queuedRow(t, 13, 0)}, ackErr: errors.New("db down")}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Equal(t, []string{"0420", "0420"}, mux.sentMTIs)
}

// A row that was sent but whose ACK write failed comes back with attempts 0; it may already be on
// the issuer's side, so it must go out as a repeat.
func TestWorker_anAdviceThatMayHaveBeenSentRepeatsAsX21__MCN_401(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	safPort := &fakeSaf{rows: []store.SafRow{sentRow(t, 14, 0)}}

	require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()))

	require.Equal(t, []string{"0421"}, mux.sentMTIs)
}

func TestWorker_claimsWithALeaseLongerThanASend__MCN_401(t *testing.T) {
	safPort := &fakeSaf{}
	w := newTestWorker(&fakeMux{}, safPort, &recordingHSM{})

	require.NoError(t, w.deliverOnce(context.Background()))

	require.Greater(t, safPort.lease, w.sendTimeout)
}

// Not signed on (no send, or the issuer answering 91 on a link still signing on) is the link being
// down, not a delivery attempt: a long outage must never dead-letter a reversal that is owed.
func TestWorker_notSignedOnNeverCountsAsAnAttempt__MCN_401(t *testing.T) {
	for name, mux := range map[string]*fakeMux{
		"send refused": {err: fmt.Errorf("send: %w", isonet.ErrNotSignedOn)},
		"issuer 91":    {response: map[int]string{39: "91"}},
	} {
		safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 11, 19)}}

		require.NoError(t, newTestWorker(mux, safPort, &recordingHSM{}).deliverOnce(context.Background()), name)

		require.Empty(t, safPort.dead, name)
		require.Equal(t, []int{19}, safPort.inFlights, name)
	}
}

// acceptAllMACs is a ResponseVerifier for tests that aren't about the response MAC.
type acceptAllMACs struct{}

func (acceptAllMACs) Verify(context.Context, string, map[int]string) bool { return true }

// recordingVerifier answers ok and records the MTI each response was verified as.
type recordingVerifier struct {
	ok   bool
	mtis []string
}

func (v *recordingVerifier) Verify(_ context.Context, mti string, _ map[int]string) bool {
	v.mtis = append(v.mtis, mti)
	return v.ok
}

func TestWorker_a0430WithABadMACIsNeverAcknowledged__NET_G20(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 1, 0)}}
	verifier := &recordingVerifier{ok: false}
	w := NewWorker(mux, fakeCards{testCardToken: testPAN}, &recordingHSM{}, hsm.StaticZAK(make([]byte, 16)), verifier, safPort, nil,
		isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)

	require.NoError(t, w.deliverOnce(context.Background()))

	require.Empty(t, safPort.acked, "a forged or corrupted 0430 must not complete a reversal")
	require.Equal(t, []int{1}, safPort.inFlights, "it is retried like any unacknowledged send")
	require.Contains(t, safPort.lastErrs[0], "MAC")
	require.Equal(t, []string{"0430"}, verifier.mtis)
}

func TestWorker_verifiesEachResponseAsItsOwnMTI__NET_G20(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00"}}
	completion, err := encodePayload(nil, advice{Fields: map[int]string{3: "000000", 7: sentDE7, 11: "000322", 37: "626514000300"}})
	require.NoError(t, err)
	safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 1, 0), {ID: 2, MTI: "0220", MaxAttempts: 20, Payload: completion}}}
	verifier := &recordingVerifier{ok: true}
	w := NewWorker(mux, fakeCards{testCardToken: testPAN}, &recordingHSM{}, hsm.StaticZAK(make([]byte, 16)), verifier, safPort, nil,
		isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)

	require.NoError(t, w.deliverOnce(context.Background()))

	require.Equal(t, []string{"0430", "0230"}, verifier.mtis, "a 0420 is answered by a 0430, a 0221 repeat by a 0230")
	require.Equal(t, []int64{1, 2}, safPort.acked)
}

// fixedMACHSM MACs every message the same, so a response carrying that MAC verifies.
type fixedMACHSM struct{ recordingHSM }

func (fixedMACHSM) ComputeMAC([]byte, []byte) ([]byte, error) {
	return []byte{1, 2, 3, 4, 5, 6, 7, 8}, nil
}

func TestWorker_checksThe0430sDE128WithTheGatewaysMACVerifier__NET_G20(t *testing.T) {
	for name, tc := range map[string]struct {
		mac   string
		acked bool
	}{
		"correct MAC": {"0102030405060708", true},
		"wrong MAC":   {"FFFFFFFFFFFFFFFF", false},
	} {
		t.Run(name, func(t *testing.T) {
			h := &fixedMACHSM{}
			zak := hsm.StaticZAK(make([]byte, 16))
			mux := &fakeMux{response: map[int]string{11: "000777", 37: "626514000124", 39: "00", 90: "0200000124092107324400000970499" + "00000000000", 128: tc.mac}}
			safPort := &fakeSaf{rows: []store.SafRow{queuedRow(t, 1, 0)}}
			w := NewWorker(mux, fakeCards{testCardToken: testPAN}, h, zak, purchase.NewMACVerifier(h, zak, nil), safPort, nil,
				isonet.Backoff{Base: time.Millisecond, Cap: 10 * time.Millisecond}, time.Millisecond)

			require.NoError(t, w.deliverOnce(context.Background()))

			require.Equal(t, tc.acked, len(safPort.acked) == 1)
		})
	}
}
