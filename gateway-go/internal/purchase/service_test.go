package purchase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeMux struct {
	linkSignedOn bool
	lastFields   map[int]string
	response     map[int]string
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
	return f.response, nil
}

type fakeTranLog struct{ rows []store.TranLogRow }

func (f *fakeTranLog) Insert(_ context.Context, row store.TranLogRow) (int64, error) {
	f.rows = append(f.rows, row)
	return int64(len(f.rows)), nil
}
func (f *fakeTranLog) UpdateStatus(_ context.Context, id int64, status string) error {
	f.rows[id-1].Status = status
	return nil
}
func (f *fakeTranLog) RecordStateTransition(context.Context, int64, string, string) error { return nil }

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

type fakeHub struct{ broadcasts int }

func (f *fakeHub) BroadcastTransaction(string, Transaction) { f.broadcasts++ }

func newTestRequest() PurchaseRequest {
	return PurchaseRequest{
		TerminalID: "00000042", CardToken: "tok_normal", EntryMode: "CHIP_PIN",
		Amount: Money{Amount: 10000, Currency: "704"},
	}
}

func TestCreatePurchase_approvedMapsToApprovedStatus__MCN_303_AC2(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "123456"}}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-1")

	require.NoError(t, err)
	require.Equal(t, "APPROVED", txn.Status)
	require.Equal(t, "123456", txn.AuthCode)
	require.Equal(t, "000000010000", mux.lastFields[4]) // DE 4 built correctly from Money
}

func TestCreatePurchase_declinedMapsToDeclinedStatus__MCN_303_AC2(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "14"}}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-2")

	require.NoError(t, err)
	require.Equal(t, "DECLINED", txn.Status)
	require.Equal(t, "14", txn.ResponseCode)
}

func TestCreatePurchase_linkDownDeclinesRc91WithoutSending__MCN_303_AC4(t *testing.T) {
	mux := &fakeMux{linkSignedOn: false}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{})

	txn, err := svc.CreatePurchase(context.Background(), newTestRequest(), "idem-key-3")

	require.NoError(t, err)
	require.Equal(t, "DECLINED", txn.Status)
	require.Equal(t, "91", txn.ResponseCode)
	require.Nil(t, mux.lastFields) // never sent
}

func TestCreatePurchase_replaysIdempotentRequest__MCN_303_AC1(t *testing.T) {
	mux := &fakeMux{linkSignedOn: true, response: map[int]string{39: "00", 38: "111111"}}
	idem := &fakeIdempotency{}
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, idem, &fakeHub{})
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
	svc := NewService(mux, DefaultCardTokens(), &fakeTranLog{}, &fakeIdempotency{}, &fakeHub{})

	req := newTestRequest()
	req.CardToken = "tok_does_not_exist"
	_, err := svc.CreatePurchase(context.Background(), req, "idem-key-4")
	require.Error(t, err)
}
