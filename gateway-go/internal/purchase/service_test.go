package purchase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeMux struct {
	linkSignedOn bool
	lastFields   map[int]string
	response     map[int]string
	err          error
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
	return f.response, f.err
}

type fakeTranLog struct{ rows []store.TranLogRow }

func (f *fakeTranLog) Insert(_ context.Context, row store.TranLogRow) (int64, error) {
	f.rows = append(f.rows, row)
	return int64(len(f.rows)), nil
}
func (f *fakeTranLog) UpdateStatus(_ context.Context, id int64, status, responseCode, authCode string) error {
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

type fakeIdempotency struct {
	stored map[string]store.StoredResponse
}

func (f *fakeIdempotency) Find(_ context.Context, key, route string) (*store.StoredResponse, error) {
	stored, ok := f.stored[key+route]
	if !ok {
		return nil, nil
	}
	return &stored, nil
}
func (f *fakeIdempotency) Store(_ context.Context, key, route, _ string, status int, body []byte) error {
	if f.stored == nil {
		f.stored = map[string]store.StoredResponse{}
	}
	f.stored[key+route] = store.StoredResponse{Status: status, Body: body}
	return nil
}

type fakeHub struct {
	broadcasts    int
	networkEvents []store.NetworkEvent
}

func (f *fakeHub) BroadcastTransaction(string, Transaction) { f.broadcasts++ }

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
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456"}}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-1")

	require.NoError(t, err)
	require.Equal(t, "APPROVED", txn.Status)
	require.Equal(t, "123456", txn.AuthCode)
	require.Equal(t, "000000010000", mux.lastFields[4]) // DE 4 built correctly from Money
}

func TestCreatePurchase_declinedMapsToDeclinedStatus__MCN_303_AC2(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "14"}}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-2")

	require.NoError(t, err)
	require.Equal(t, "DECLINED", txn.Status)
	require.Equal(t, "14", txn.ResponseCode)
}

func TestCreatePurchase_linkDownDeclinesRc91WithoutSending__MCN_303_AC4(t *testing.T) {
	mux := &fakeMux{linkSignedOn: false}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-3")

	require.NoError(t, err)
	require.Equal(t, "DECLINED", txn.Status)
	require.Equal(t, "91", txn.ResponseCode)
	require.Nil(t, mux.lastFields) // never sent
}

func TestCreatePurchase_replaysIdempotentRequest__MCN_303_AC1(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "111111"}}
	idem := &fakeIdempotency{}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, idem, &fakeHub{}, &fakeReversal{})
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
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, &fakeReversal{})

	req := newTestRequest()
	req.CardToken = "tok_does_not_exist"
	_, err := svc.CreatePurchase(context.Background(), req, "idem-key-4")
	require.Error(t, err)
}

func TestCreatePurchase_timeoutQueuesReversalWithReasonSixtyEight__MCN_401_AC1(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, err: context.DeadlineExceeded}
	reversal := &fakeReversal{}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, reversal)

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-timeout")

	require.NoError(t, err)
	require.Equal(t, "TIMED_OUT", txn.Status)
	require.Equal(t, []string{reasonTimeout}, reversal.calls)
}

func TestCreatePurchase_timeoutReturnsErrorWhenReversalQueueingFails__MCN_401_AC1(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, err: context.DeadlineExceeded}
	reversal := &fakeReversal{err: errors.New("boom")}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{}, reversal)

	_, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-timeout-fail")
	require.Error(t, err)
}

func TestCancelPurchase_queuesReversalWithReasonSeventeen__MCN_401_AC5(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "x", Status: statusSent, Amount: 5000, Currency: "704"}}}
	reversal := &fakeReversal{}
	svc := NewService(&fakeMux{linkSignedOn: true}, DefaultCardTokens(), tranLog, &fakeIdempotency{}, &fakeHub{}, reversal)

	txn, err := svc.CancelPurchase(context.Background(), "x", "cancel-key-1")

	require.NoError(t, err)
	require.Equal(t, "REVERSAL_PENDING", txn.Status)
	require.Equal(t, []string{reasonCancellation}, reversal.calls)
}

func TestCancelPurchase_replaysIdempotentRequest__MCN_401_AC5(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "y", Status: statusSent, Amount: 5000, Currency: "704"}}}
	reversal := &fakeReversal{}
	idem := &fakeIdempotency{}
	svc := NewService(&fakeMux{linkSignedOn: true}, DefaultCardTokens(), tranLog, idem, &fakeHub{}, reversal)

	first, err := svc.CancelPurchase(context.Background(), "y", "same-cancel-key")
	require.NoError(t, err)
	second, err := svc.CancelPurchase(context.Background(), "y", "same-cancel-key")
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Len(t, reversal.calls, 1) // second call was a replay, not a re-queue
}

func TestRecordLateResponse_setsColumnsWithoutChangingStatus__MCN_403_AC1(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "z", Status: "TIMED_OUT"}}}
	hub := &fakeHub{}
	svc := NewService(&fakeMux{}, DefaultCardTokens(), tranLog, &fakeIdempotency{}, hub, &fakeReversal{})

	err := svc.RecordLateResponse(context.Background(), "z", "00")

	require.NoError(t, err)
	require.Equal(t, "TIMED_OUT", tranLog.rows[0].Status)
	require.Equal(t, "00", tranLog.rows[0].LateResponseCode)
	require.Len(t, hub.networkEvents, 1)
	require.Equal(t, "WARN", hub.networkEvents[0].Severity)
}

func TestRecordLateResponse_leavesOnTimeResponseAlone__MCN_403_AC1(t *testing.T) {
	tranLog := &fakeTranLog{rows: []store.TranLogRow{{RRN: "w", Status: statusSent}}}
	hub := &fakeHub{}
	svc := NewService(&fakeMux{}, DefaultCardTokens(), tranLog, &fakeIdempotency{}, hub, &fakeReversal{})

	err := svc.RecordLateResponse(context.Background(), "w", "00")

	require.NoError(t, err)
	require.Empty(t, tranLog.rows[0].LateResponseCode)
	require.Empty(t, hub.networkEvents)
}
