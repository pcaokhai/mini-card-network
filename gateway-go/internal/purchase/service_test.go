package purchase

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/store"
)

type fakeMux struct {
	linkSignedOn bool
	lastFields   map[int]string
	response     map[int]string
	err          error
	onSend       func() // runs inside Send, e.g. to cancel the caller's context mid-flight
}

func (f *fakeMux) IsSignedOn() bool { return f.linkSignedOn }

func (f *fakeMux) NextSTAN() (string, bool) {
	if !f.linkSignedOn {
		return "", false
	}
	return "000001", true
}

func (f *fakeMux) Send(_ context.Context, _ string, fields map[int]string) (map[int]string, error) {
	f.lastFields = fields
	if f.onSend != nil {
		f.onSend()
	}
	return f.response, f.err
}

type fakeTranLog struct {
	rows       []store.TranLogRow
	failStatus string // UpdateStatus to this status fails, leaving the row as it was
}

func (f *fakeTranLog) Insert(_ context.Context, row store.TranLogRow) (int64, error) {
	f.rows = append(f.rows, row)
	return int64(len(f.rows)), nil
}
func (f *fakeTranLog) UpdateStatus(_ context.Context, id int64, status, responseCode, authCode string) error {
	if status == f.failStatus {
		return errors.New("db down")
	}
	f.rows[id-1].Status = status
	f.rows[id-1].ResponseCode = responseCode
	f.rows[id-1].AuthCode = authCode
	return nil
}
func (f *fakeTranLog) RecordStateTransition(context.Context, int64, string, string) error { return nil }

func (f *fakeTranLog) UpdateLateResponse(_ context.Context, rrn, responseCode string) error {
	for i := range f.rows {
		if f.rows[i].RRN == rrn {
			f.rows[i].LateResponseCode = responseCode
			at := time.Now().UTC()
			f.rows[i].LateResponseAt = &at
			return nil
		}
	}
	return store.ErrNotFound
}

func (f *fakeTranLog) Get(_ context.Context, rrn string) (store.TranLogRow, error) {
	for _, row := range f.rows {
		if row.RRN == rrn {
			return row, nil
		}
	}
	return store.TranLogRow{}, store.ErrNotFound
}

type fakeReversal struct {
	calls  []string
	queued []store.TranLogRow
	err    error
}

func (f *fakeReversal) Queue(_ context.Context, txn store.TranLogRow, reasonCode string) error {
	f.calls = append(f.calls, reasonCode)
	f.queued = append(f.queued, txn)
	return f.err
}

// fakeHSM is a test double for hsm.Module: purchase.Service only calls ComputeMAC.
type fakeHSM struct {
	macToReturn []byte
	macErr      error
	computedFor [][]byte
	// macForKey, if set, overrides macToReturn per-zak (keyed by hex.EncodeToString(zak)) -
	// used by the MCN-504-AC2 dual-key test to give the retired ZAK a different valid MAC.
	macForKey map[string][]byte
}

func (f *fakeHSM) WrapUnderLMK([]byte) ([]byte, error)                 { return nil, nil }
func (f *fakeHSM) Unwrap(k []byte) ([]byte, error)                     { return k, nil } // identity: tests pass the clear key straight through
func (f *fakeHSM) ComputeKCV([]byte) (string, error)                   { return "", nil }
func (f *fakeHSM) TranslatePIN([]byte, []byte, []byte) ([]byte, error) { return nil, nil }

func (f *fakeHSM) ComputeMAC(packedMessageExcludingMACField []byte, zak []byte) ([]byte, error) {
	f.computedFor = append(f.computedFor, packedMessageExcludingMACField)
	if mac, ok := f.macForKey[hex.EncodeToString(zak)]; ok {
		return mac, nil
	}
	return f.macToReturn, f.macErr
}

// fakeRetiredKeyFinder is a test double for RetiredKeyFinder (MCN-504-AC2).
type fakeRetiredKeyFinder struct{ row *store.KeyRow }

func (f *fakeRetiredKeyFinder) FindRecentlyRetired(context.Context, string, string, time.Duration) (*store.KeyRow, error) {
	return f.row, nil
}

// testZAK satisfies hsm.Module.ComputeMAC's 16-byte length check; fakeHSM ignores its value.
var testZAK = make([]byte, 16)

// stdMACHex is hex.EncodeToString(stdHSM()'s fixed MAC) - tests that don't care about MAC
// verification set their fakeMux response's DE 64 to this so verifyIncomingMAC still passes.
const stdMACHex = "0102030405060708"

func stdHSM() *fakeHSM { return &fakeHSM{macToReturn: []byte{1, 2, 3, 4, 5, 6, 7, 8}} }

// fakeIdempotency mirrors store.IdempotencyRepository's reserve/store/release contract.
type fakeIdempotency struct {
	stored   map[string]store.StoredResponse
	storeCtx context.Context // the context the last Store ran on
	hashes   map[string]string
	pending  map[string]bool
	released []string
	rrns     map[string]string    // AttachRRN, by key+route
	reserved map[string]time.Time // when each pending key was reserved
}

// pendAt makes key a reservation made at reservedAt that sent rrn, as a crashed request leaves it.
func (f *fakeIdempotency) pendAt(key, route, hash, rrn string, reservedAt time.Time) {
	_, _ = f.Reserve(context.Background(), key, route, hash)
	k := key + route
	f.rrns[k], f.reserved[k] = rrn, reservedAt
}

func (f *fakeIdempotency) AttachRRN(_ context.Context, key, route, rrn string) error {
	f.rrns[key+route] = rrn
	return nil
}

func (f *fakeIdempotency) Reserve(_ context.Context, key, route, hash string) (*store.StoredResponse, error) {
	if f.hashes == nil {
		f.stored, f.hashes, f.pending = map[string]store.StoredResponse{}, map[string]string{}, map[string]bool{}
		f.rrns, f.reserved = map[string]string{}, map[string]time.Time{}
	}
	k := key + route
	if h, ok := f.hashes[k]; ok {
		switch {
		case h != hash:
			return nil, store.ErrIdempotencyKeyMismatch
		case f.pending[k]:
			return nil, &store.InProgressError{RRN: f.rrns[k], ReservedAt: f.reserved[k]}
		}
		stored := f.stored[k]
		return &stored, nil
	}
	f.hashes[k], f.pending[k], f.reserved[k] = hash, true, time.Now()
	return nil, nil
}
func (f *fakeIdempotency) Store(ctx context.Context, key, route, hash string, status int, body []byte) error {
	f.storeCtx = ctx
	if ctx.Err() != nil {
		return ctx.Err()
	}
	k := key + route
	f.stored[k], f.hashes[k] = store.StoredResponse{Status: status, Body: body}, hash
	delete(f.pending, k)
	return nil
}

func (f *fakeIdempotency) Release(_ context.Context, key, route string) error {
	k := key + route
	if f.pending[k] {
		delete(f.pending, k)
		delete(f.hashes, k)
		f.released = append(f.released, key)
	}
	return nil
}

type fakeEvents struct{ recorded []string }

func (f *fakeEvents) RecordEvent(_ context.Context, severity, _, technicalText string) error {
	f.recorded = append(f.recorded, severity+" "+technicalText)
	return nil
}

type fakeHub struct {
	broadcasts    int
	eventTypes    []string
	last          Transaction
	networkEvents []store.NetworkEvent
}

func (f *fakeHub) BroadcastTransaction(eventType string, txn Transaction) {
	f.broadcasts++
	f.eventTypes = append(f.eventTypes, eventType)
	f.last = txn
}

func (f *fakeHub) BroadcastNetworkEvent(evt store.NetworkEvent) {
	f.networkEvents = append(f.networkEvents, evt)
}

func newTestRequest() PurchaseRequest {
	return PurchaseRequest{
		TerminalID: "00000042", CardToken: "tok_normal", EntryMode: "CHIP_PIN",
		Amount: Money{Amount: 10000, Currency: "704"},
	}
}

func TestCreatePurchase_approvedMapsToApprovedStatus__MCN_303_AC2(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456", 64: stdMACHex}}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-1")

	require.NoError(t, err)
	require.Equal(t, "APPROVED", txn.Status)
	require.Equal(t, "123456", txn.AuthCode)
	require.Equal(t, "000000010000", mux.lastFields[4]) // DE 4 built correctly from Money
}

func TestCreatePurchase_declinedMapsToDeclinedStatus__MCN_303_AC2(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "14", 64: stdMACHex}}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-2")

	require.NoError(t, err)
	require.Equal(t, "DECLINED", txn.Status)
	require.Equal(t, "14", txn.ResponseCode)
}

func TestCreatePurchase_linkDownDeclinesRc91WithoutSending__MCN_303_AC4(t *testing.T) {
	mux := &fakeMux{linkSignedOn: false}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-3")

	require.NoError(t, err)
	require.Equal(t, "DECLINED", txn.Status)
	require.Equal(t, "91", txn.ResponseCode)
	require.Nil(t, mux.lastFields) // never sent
}

func TestCreatePurchase_replaysIdempotentRequest__MCN_303_AC1(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "111111", 64: stdMACHex}}
	idem := &fakeIdempotency{}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, idem, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})
	req := PurchaseRequest{TerminalID: "00000042", CardToken: "tok_normal", EntryMode: "CHIP_PIN", Amount: Money{Amount: 5000, Currency: "704"}}

	first, err := svc.CreatePurchase(context.Background(), req, "same-key")
	require.NoError(t, err)

	mux.response = map[int]string{39: "05"} // if this were sent again, it'd differ - proving no resend
	second, err := svc.CreatePurchase(context.Background(), req, "same-key")
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestCreatePurchase_unknownCardTokenIsDeclined__MCN_303(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00"}}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})

	req := newTestRequest()
	req.CardToken = "tok_does_not_exist"
	_, err := svc.CreatePurchase(context.Background(), req, "idem-key-4")
	require.Error(t, err)
}

func TestCreatePurchase_timeoutQueuesReversalWithReasonSixtyEight__MCN_401_AC1(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, err: context.DeadlineExceeded}
	reversal := &fakeReversal{}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, reversal, stdHSM(), testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-timeout")

	require.NoError(t, err)
	require.Equal(t, "REVERSAL_PENDING", txn.Status, "POS-G9: the response reports the row's real status")
	require.Equal(t, []string{reasonTimeout}, reversal.calls)
}

func TestCreatePurchase_everyPostSendFailureQueuesReversal__POS_G4(t *testing.T) {
	failures := map[string]error{
		"timeout":           context.DeadlineExceeded,
		"request cancelled": context.Canceled,
		"broken pipe":       fmt.Errorf("write request: %w", syscall.EPIPE),
		"connection closed": io.EOF,
	}
	for name, sendErr := range failures {
		t.Run(name, func(t *testing.T) {
			mux := &fakeMux{linkSignedOn: true, err: sendErr}
			reversal := &fakeReversal{}
			tranLog := &fakeTranLog{}
			svc := NewService(mux, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, reversal, stdHSM(), testZAK, nil, &fakeEvents{})

			txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-"+name)

			require.NoError(t, err, "an unknown outcome is not an HTTP error")
			require.Equal(t, "REVERSAL_PENDING", txn.Status)
			require.Equal(t, []string{reasonTimeout}, reversal.calls)
			require.Equal(t, statusTimedOut, reversal.queued[0].Status, "queued from the state the row holds")
			require.Equal(t, statusTimedOut, tranLog.rows[0].Status)
		})
	}
}

func TestCreatePurchase_linkDroppedBeforeSendDeclinesRc91WithoutReversal__POS_G4(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, err: isonet.ErrNotSignedOn}
	reversal := &fakeReversal{}
	tranLog := &fakeTranLog{}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, reversal, stdHSM(), testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-link-dropped")

	require.NoError(t, err)
	require.Equal(t, "DECLINED", txn.Status)
	require.Equal(t, "91", txn.ResponseCode)
	require.Empty(t, reversal.calls, "nothing left the gateway, so nothing to reverse")
	require.Equal(t, "DECLINED", tranLog.rows[0].Status)
}

func TestCreatePurchase_timeoutWhoseReversalCantBeQueuedReportsTheRowAsIs__MCN_401_AC1(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, err: context.DeadlineExceeded}
	reversal := &fakeReversal{err: errors.New("boom")}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, reversal, stdHSM(), testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-timeout-fail")

	require.NoError(t, err, "answered from tran_log (S1); the sweeper queues the reversal later (POS-G16)")
	require.Equal(t, statusTimedOut, txn.Status, "never REVERSAL_PENDING when nothing was queued")
}

func TestCancelPurchase_queuesReversalWithReasonSeventeen__MCN_401_AC5(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "x", Type: tranTypePurchase, Status: statusApproved, Amount: 5000, Currency: "704"}}}
	reversal := &fakeReversal{}
	svc := NewService(&fakeMux{linkSignedOn: true}, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, reversal, stdHSM(), testZAK, nil, &fakeEvents{})

	txn, err := svc.CancelPurchase(context.Background(), "x", "cancel-key-1")

	require.NoError(t, err)
	require.Equal(t, "REVERSAL_PENDING", txn.Status)
	require.Equal(t, []string{reasonCancellation}, reversal.calls)
}

func TestCancelPurchase_replaysIdempotentRequest__MCN_401_AC5(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "y", Type: tranTypePurchase, Status: statusApproved, Amount: 5000, Currency: "704"}}}
	reversal := &fakeReversal{}
	idem := &fakeIdempotency{}
	svc := NewService(&fakeMux{linkSignedOn: true}, DefaultCardTokens(), testMerchants, tranLog, idem, &fakeHub{}, reversal, stdHSM(), testZAK, nil, &fakeEvents{})

	first, err := svc.CancelPurchase(context.Background(), "y", "same-cancel-key")
	require.NoError(t, err)
	second, err := svc.CancelPurchase(context.Background(), "y", "same-cancel-key")
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Len(t, reversal.calls, 1) // second call was a replay, not a re-queue
}

// Only an approved purchase holds the cardholder's money; a decline (including a link-down one
// that was never sent) or a purchase already being reversed has nothing to cancel.
func TestCancelPurchase_onlyAnApprovedPurchaseCanBeCancelled__MCN_401(t *testing.T) {
	for _, status := range []string{statusDeclined, statusTimedOut, statusReversalPending, "REVERSED", statusSent} {
		tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "v", Status: status}}}
		reversal := &fakeReversal{}
		svc := NewService(&fakeMux{linkSignedOn: true}, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, reversal, stdHSM(), testZAK, nil, &fakeEvents{})

		_, err := svc.CancelPurchase(context.Background(), "v", "cancel-"+status)

		require.ErrorIs(t, err, store.ErrNotReversible, status)
		require.Empty(t, reversal.calls, status)
	}
}

// A pre-auth or completion is reversed by its own flow: a 0420 naming a 0200 original would find
// nothing at the issuer, be acknowledged anyway, and leave the hold in place.
func TestCancelPurchase_refusesAnApprovedNonPurchase__MCN_401(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "p", Type: "PREAUTH", Status: statusApproved}}}
	reversal := &fakeReversal{}
	svc := NewService(&fakeMux{linkSignedOn: true}, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, reversal, stdHSM(), testZAK, nil, &fakeEvents{})

	_, err := svc.CancelPurchase(context.Background(), "p", "cancel-preauth")

	require.ErrorIs(t, err, store.ErrNotReversible)
	require.Empty(t, reversal.calls)
}

func TestRecordLateResponse_setsColumnsWithoutChangingStatus__MCN_403_AC1(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "z", Status: statusTimedOut}}}
	hub := &fakeHub{}
	events := &fakeEvents{}
	svc := NewService(&fakeMux{}, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, hub, &fakeReversal{}, stdHSM(), testZAK, nil, events)

	err := svc.RecordLateResponse(context.Background(), "z", "00")

	require.NoError(t, err)
	require.Equal(t, statusTimedOut, tranLog.rows[0].Status)
	require.Equal(t, "00", tranLog.rows[0].LateResponseCode)
	require.Len(t, hub.networkEvents, 1)
	require.Equal(t, "WARN", hub.networkEvents[0].Severity)
	require.Equal(t, []string{"WARN late 0210 for RRN z, RC 00"}, events.recorded, "NET-G5: the event is persisted, not only broadcast")
	require.Equal(t, []string{eventUpdated}, hub.eventTypes, "OVW-G3")
}

func TestRecordLateResponse_leavesOnTimeResponseAlone__MCN_403_AC1(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "w", Status: statusSent}}}
	hub := &fakeHub{}
	svc := NewService(&fakeMux{}, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, hub, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})

	err := svc.RecordLateResponse(context.Background(), "w", "00")

	require.NoError(t, err)
	require.Empty(t, tranLog.rows[0].LateResponseCode)
	require.Empty(t, hub.networkEvents)
}

func TestSendPurchase_computesAndAttachesMACOnOutgoing0200__MCN_502_AC1(t *testing.T) {
	hsmFake := stdHSM()
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456", 64: stdMACHex}}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, hsmFake, testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-mac-1")

	require.NoError(t, err)
	require.Equal(t, "APPROVED", txn.Status)
	require.Equal(t, stdMACHex, mux.lastFields[64])
	require.NotEmpty(t, hsmFake.computedFor)
}

func TestSendPurchase_badIncomingMACFailsTransactionAndQueuesReversal__MCN_502_AC3(t *testing.T) {
	hsmFake := stdHSM()
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456", 64: badMACHex}}
	reversal := &fakeReversal{}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, reversal, hsmFake, testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-mac-2")

	require.NoError(t, err)
	require.Equal(t, "96", txn.ResponseCode)
	require.Equal(t, "DECLINED", txn.Status)
	require.Equal(t, []string{reasonMacFailure}, reversal.calls)
}

func TestSendPurchase_missingIncomingMACFailsTransaction__MCN_502_AC3(t *testing.T) {
	hsmFake := stdHSM()
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456"}}
	reversal := &fakeReversal{}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, reversal, hsmFake, testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-mac-3")

	require.NoError(t, err)
	require.Equal(t, "96", txn.ResponseCode)
	require.Equal(t, []string{reasonMacFailure}, reversal.calls)
}

func TestSendPurchase_incomingMACVerifiesAgainstRecentlyRetiredZAKDuringRotationWindow__MCN_504_AC2(t *testing.T) {
	oldZAK := []byte("old-zak-test-key")
	oldMACHex := "aabbccddeeff0011"
	hsmFake := &fakeHSM{
		macToReturn: []byte{1, 2, 3, 4, 5, 6, 7, 8}, // does not match the response's DE 64
		macForKey:   map[string][]byte{hex.EncodeToString(oldZAK): {0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11}},
	}
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456", 64: oldMACHex}}
	reversal := &fakeReversal{}
	keyStore := &fakeRetiredKeyFinder{row: &store.KeyRow{KeyUnderLMKHex: hex.EncodeToString(oldZAK)}}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, reversal, hsmFake, testZAK, keyStore, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-rotation-1")

	require.NoError(t, err)
	require.NotEqual(t, "96", txn.ResponseCode) // verified on the fallback retired-key retry, not rejected
	require.Empty(t, reversal.calls)
}

func TestSendPurchase_incomingMACStillFailsWhenNoRecentlyRetiredZAKMatches__MCN_504_AC2(t *testing.T) {
	hsmFake := stdHSM()
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456", 64: badMACHex}}
	reversal := &fakeReversal{}
	keyStore := &fakeRetiredKeyFinder{row: nil}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, reversal, hsmFake, testZAK, keyStore, &fakeEvents{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-rotation-2")

	require.NoError(t, err)
	require.Equal(t, "96", txn.ResponseCode)
	require.Equal(t, []string{reasonMacFailure}, reversal.calls)
}

// fakeMerchants resolves terminals from a fixed table; anything else is an unknown terminal.
type fakeMerchants map[string]store.Merchant

func (f fakeMerchants) Merchant(_ context.Context, tid string) (store.Merchant, error) {
	if m, ok := f[tid]; ok {
		return m, nil
	}
	return store.Merchant{}, store.ErrUnknownTerminal
}

var testMerchants = fakeMerchants{
	"00000042": {MID: "GOCPHO000000001", Name: "Cà phê Góc Phố"},
	"00000043": {MID: "ANHDUO000000001", Name: "Nhà sách Ánh Dương"},
}

func TestCreatePurchase_usesTerminalsMerchantInDE42AndTranLog__MCN_002(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456", 64: stdMACHex}}
	tranLog := &fakeTranLog{}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})
	req := newTestRequest()
	req.TerminalID = "00000043"

	txn, err := svc.CreatePurchase(context.Background(), req, "idem-merchant")

	require.NoError(t, err)
	require.Equal(t, "ANHDUO000000001", mux.lastFields[42])
	require.Equal(t, "ANHDUO000000001", tranLog.rows[0].MerchantID)
	require.Equal(t, "Nhà sách Ánh Dương", txn.MerchantName)
}

func TestCreatePurchase_unknownTerminalSendsNothing__MCN_002(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true}
	tranLog := &fakeTranLog{}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})
	req := newTestRequest()
	req.TerminalID = "99999999"

	_, err := svc.CreatePurchase(context.Background(), req, "idem-unknown-terminal")

	require.ErrorIs(t, err, store.ErrUnknownTerminal)
	require.Nil(t, mux.lastFields)
	require.Empty(t, tranLog.rows)
}

func TestCreatePurchase_linkDownDeclineKeepsTerminalsMerchant__MCN_002(t *testing.T) {
	tranLog := &fakeTranLog{}
	svc := NewService(&fakeMux{linkSignedOn: false}, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})
	req := newTestRequest()
	req.TerminalID = "00000043"

	txn, err := svc.CreatePurchase(context.Background(), req, "idem-linkdown-merchant")

	require.NoError(t, err)
	require.Equal(t, "ANHDUO000000001", tranLog.rows[0].MerchantID)
	require.Equal(t, "Nhà sách Ánh Dương", txn.MerchantName)
}

func TestCreatePurchase_recordsWhatA0420MustRepeat__MCN_401(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456", 64: stdMACHex}}
	tranLog := &fakeTranLog{}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})

	_, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-0420-fields")

	require.NoError(t, err)
	row := tranLog.rows[0]
	require.Equal(t, "tok_normal", row.CardToken)
	require.Equal(t, "0200", row.MTI)
	require.Equal(t, mux.lastFields[3], row.ProcessingCode)
	require.Equal(t, mux.lastFields[22], row.POSEntryMode)
	require.NotNil(t, row.SentAt)
	require.Equal(t, mux.lastFields[7], row.SentAt.UTC().Format("0102150405"), "DE 90 repeats the DE 7 that was sent")
}

func TestCreatePurchase_callerGivingUpMidSendStillStoresTheOutcome__POS_G4(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456", 64: stdMACHex}, onSend: cancel}
	idem := &fakeIdempotency{}
	svc := NewService(mux, DefaultCardTokens(), testMerchants, &fakeTranLog{}, idem, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})

	txn, err := svc.CreatePurchase(ctx, newTestRequest(), "idem-gave-up")

	require.NoError(t, err)
	require.Equal(t, "APPROVED", txn.Status)
	require.Len(t, idem.stored, 1, "a retry with the same key must replay, never resend the 0200")
	_, hasDeadline := idem.storeCtx.Deadline()
	require.True(t, hasDeadline, "work after the send is detached from the caller but still bounded")
}

func TestCreatePurchase_linkDroppedBeforeWriteLogsOneRow__POS_G4(t *testing.T) {
	tranLog := &fakeTranLog{}
	svc := NewService(&fakeMux{linkSignedOn: true, err: isonet.ErrNotSignedOn}, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{}, stdHSM(), testZAK, nil, &fakeEvents{})

	_, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-dropped-once")

	require.NoError(t, err)
	require.Len(t, tranLog.rows, 1)
}

func TestCreatePurchase_reversalIsQueuedEvenWhenTheStatusUpdateFails__POS_G4(t *testing.T) {
	for name, tc := range map[string]struct {
		mux    *fakeMux
		fail   string
		reason string
	}{
		"timeout": {&fakeMux{linkSignedOn: true, err: context.DeadlineExceeded}, statusTimedOut, reasonTimeout},
		"bad MAC": {&fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 64: badMACHex}}, "DECLINED", reasonMacFailure},
	} {
		t.Run(name, func(t *testing.T) {
			reversal := &fakeReversal{}
			tranLog := &fakeTranLog{failStatus: tc.fail}
			svc := NewService(tc.mux, DefaultCardTokens(), testMerchants, tranLog, &fakeIdempotency{}, &fakeHub{}, reversal, stdHSM(), testZAK, nil, &fakeEvents{})

			_, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-status-fails")

			require.NoError(t, err, "a failure after the send is answered from tran_log (S1)")
			require.Equal(t, []string{tc.reason}, reversal.calls)
			require.Equal(t, "SENT", reversal.queued[0].Status, "queued from the state the row still holds")
		})
	}
}

// badMACHex never matches the fake HSM's MAC.
const badMACHex = "FFFFFFFFFFFFFFFF"

func newIdemTestService(mux *fakeMux, tranLog *fakeTranLog, idem *fakeIdempotency, hub *fakeHub, reversal *fakeReversal) *Service {
	return NewService(mux, DefaultCardTokens(), testMerchants, tranLog, idem, hub, reversal, stdHSM(), testZAK, nil, &fakeEvents{})
}

func TestCreatePurchase_sameKeyDifferentRequestIsAMismatch__POS_G3(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "111111", 64: stdMACHex}}
	svc := newIdemTestService(mux, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{})
	_, err := svc.CreatePurchase(context.Background(), newTestRequest(), "same-key")
	require.NoError(t, err)
	mux.lastFields = nil

	other := newTestRequest()
	other.Amount.Amount = 99
	_, err = svc.CreatePurchase(context.Background(), other, "same-key")

	require.ErrorIs(t, err, store.ErrIdempotencyKeyMismatch)
	require.Nil(t, mux.lastFields, "never sent")
}

func TestCreatePurchase_releasesTheKeyOnlyWhenNothingWasSent__POS_G3(t *testing.T) {
	idem := &fakeIdempotency{}
	svc := newIdemTestService(&fakeMux{linkSignedOn: true}, &fakeTranLog{}, idem, &fakeHub{}, &fakeReversal{})
	req := newTestRequest()
	req.CardToken = "tok_does_not_exist"
	_, err := svc.CreatePurchase(context.Background(), req, "key-unknown-card")
	require.Error(t, err)
	require.Equal(t, []string{"key-unknown-card"}, idem.released, "nothing was sent, so the key may be retried")

	svc = newIdemTestService(&fakeMux{linkSignedOn: true, err: context.DeadlineExceeded}, &fakeTranLog{failStatus: statusTimedOut}, idem, &fakeHub{}, &fakeReversal{err: errors.New("boom")})
	_, _ = svc.CreatePurchase(context.Background(), newTestRequest(), "key-sent")
	require.NotContains(t, idem.released, "key-sent", "the request may be at the issuer: a retry must never resend it")
}

func TestCreatePurchase_responseCarriesLabelLatencyAndTraceID__POS_G8(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "111111", 64: stdMACHex}}
	tranLog := &fakeTranLog{}
	svc := newIdemTestService(mux, tranLog, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{})

	txn, err := svc.CreatePurchase(tracedContext(), newTestRequest(), "key-trace")

	require.NoError(t, err)
	require.NotNil(t, txn.ResponseLabel)
	require.NotNil(t, txn.LatencyMs)
	require.Equal(t, testTraceID, txn.TraceID)
	require.Equal(t, testTraceID, tranLog.rows[0].TraceID)
}

func TestCreatePurchase_responseWithoutRCIsDeclinedFormatError__OVW_G11(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{64: stdMACHex}}
	svc := newIdemTestService(mux, &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "key-no-rc")

	require.NoError(t, err)
	require.Equal(t, statusDeclined, txn.Status)
	require.Equal(t, "30", txn.ResponseCode)
}

func TestCancelPurchase_broadcastsTheStatusChange__OVW_G3(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "626514000001", Type: tranTypePurchase, Status: statusApproved, Amount: 5000, Currency: "704"}}}
	hub := &fakeHub{}
	svc := newIdemTestService(&fakeMux{linkSignedOn: true}, tranLog, &fakeIdempotency{}, hub, &fakeReversal{})

	_, err := svc.CancelPurchase(context.Background(), "626514000001", "key-cancel")

	require.NoError(t, err)
	require.Equal(t, []string{eventUpdated}, hub.eventTypes)
}

func TestBroadcastUpdate_sendsTheRowsCurrentState__OVW_G3(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "626514000002", Type: tranTypePurchase, Status: "REVERSED", Amount: 5000, Currency: "704", ResponseCode: "00", MaskedPAN: "970436******4417", ReversalReasonCode: "68"}}}
	hub := &fakeHub{}
	svc := newIdemTestService(&fakeMux{}, tranLog, &fakeIdempotency{}, hub, &fakeReversal{})

	require.NoError(t, svc.BroadcastUpdate(context.Background(), "626514000002"))

	require.Equal(t, []string{eventUpdated}, hub.eventTypes)
	require.Equal(t, "REVERSED", hub.last.Status)
	require.Equal(t, "970436******4417", hub.last.MaskedPAN)
	require.Equal(t, store.ReversalTimeout, hub.last.ReversalReason)
}

const testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

func tracedContext() context.Context {
	traceID, _ := trace.TraceIDFromHex(testTraceID)
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	return trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID}))
}

func TestCreatePurchase_aRetryOfAStaleKeyAnswersFromTheTransaction__S1(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "626514000801", Type: tranTypePurchase, Status: statusApproved, ResponseCode: "00", Amount: 10000, Currency: "704", MaskedPAN: "970436******4417"}}}
	idem := &fakeIdempotency{}
	idem.pendAt("key-crashed", purchaseRoute, hashRequest(newTestRequest()), "626514000801", time.Now().Add(-2*time.Minute))
	mux := &fakeMux{linkSignedOn: true}
	svc := newIdemTestService(mux, tranLog, idem, &fakeHub{}, &fakeReversal{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "key-crashed")

	require.NoError(t, err, "a retry learns the outcome instead of a 409 for 24 h")
	require.Equal(t, statusApproved, txn.Status)
	require.Equal(t, "626514000801", txn.RRN)
	require.Nil(t, mux.lastFields, "never resent")
	require.Contains(t, idem.stored, "key-crashed"+purchaseRoute, "the answer becomes the key's response")
}

func TestCreatePurchase_aRetryWhileTheFirstIsStillInFlightIsAConflict__S1(t *testing.T) {
	idem := &fakeIdempotency{}
	idem.pendAt("key-busy", purchaseRoute, hashRequest(newTestRequest()), "626514000802", time.Now())
	svc := newIdemTestService(&fakeMux{linkSignedOn: true}, &fakeTranLog{}, idem, &fakeHub{}, &fakeReversal{})

	_, err := svc.CreatePurchase(context.Background(), newTestRequest(), "key-busy")

	require.ErrorIs(t, err, store.ErrIdempotencyInProgress)
}

func TestCreatePurchase_aFailureAfterTheSendAnswersFromTheTransaction__S1(t *testing.T) {
	idem := &fakeIdempotency{}
	tranLog := &fakeTranLog{}
	svc := newIdemTestService(&fakeMux{linkSignedOn: true, err: context.DeadlineExceeded}, tranLog, idem, &fakeHub{}, &fakeReversal{err: errors.New("db down")})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "key-after-send")

	require.NoError(t, err)
	require.Equal(t, statusTimedOut, txn.Status, "the row's real status")
	require.Equal(t, tranLog.rows[0].RRN, idem.rrns["key-after-send"+purchaseRoute], "the RRN was attached before the send")
}
