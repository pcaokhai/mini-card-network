package advtxn

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

type fakeMux struct {
	response    map[int]string
	err         error
	lastMTI     string
	lastFields  map[int]string
	stanCounter int
}

func (f *fakeMux) NextSTAN() (string, bool) {
	f.stanCounter++
	return "000001", true
}

func (f *fakeMux) Send(_ context.Context, mti string, fields map[int]string) (map[int]string, error) {
	f.lastMTI = mti
	f.lastFields = fields
	return f.response, f.err
}

type fakeTranLog struct{ rows []store.TranLogRow }

func (f *fakeTranLog) Insert(_ context.Context, row store.TranLogRow) (int64, error) {
	f.rows = append(f.rows, row)
	return int64(len(f.rows)), nil
}

func (f *fakeTranLog) Get(_ context.Context, rrn string) (store.TranLogRow, error) {
	for _, row := range f.rows {
		if row.RRN == rrn {
			return row, nil
		}
	}
	return store.TranLogRow{}, store.ErrNotFound
}

func (f *fakeTranLog) UpdateStatus(_ context.Context, id int64, status, responseCode, authCode string) error {
	f.rows[id-1].Status = status
	f.rows[id-1].ResponseCode = responseCode
	f.rows[id-1].AuthCode = authCode
	return nil
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

type fakeHub struct{ broadcasts int }

func (f *fakeHub) BroadcastTransaction(string, Transaction) { f.broadcasts++ }

type fakeHSM struct{}

func (fakeHSM) WrapUnderLMK([]byte) ([]byte, error)                 { return nil, nil }
func (fakeHSM) Unwrap([]byte) ([]byte, error)                       { return nil, nil }
func (fakeHSM) ComputeKCV([]byte) (string, error)                   { return "", nil }
func (fakeHSM) TranslatePIN([]byte, []byte, []byte) ([]byte, error) { return nil, nil }
func (fakeHSM) ComputeMAC(_ []byte, _ []byte) ([]byte, error) {
	return []byte{1, 2, 3, 4, 5, 6, 7, 8}, nil
}

var _ hsm.Module = fakeHSM{}

var testZAK = make([]byte, 16)

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
	"00000047": {MID: "BANHMA000000001", Name: "Tiệm bánh Mây"},
}

func newTestService(mux *fakeMux) (*Service, *fakeTranLog, *fakeIdempotency, *fakeHub) {
	tranLog := &fakeTranLog{}
	idem := &fakeIdempotency{}
	hub := &fakeHub{}
	svc := NewService(mux, purchase.DefaultCardTokens(), testMerchants, tranLog, idem, hub, fakeHSM{}, testZAK)
	return svc, tranLog, idem, hub
}

func samplePreAuthRequest() PreAuthRequest {
	return PreAuthRequest{
		CardPresentData: CardPresentData{TerminalID: "00000042", CardToken: "tok_normal", EntryMode: "CHIP_PIN"},
		Amount:          Money{Amount: 10000, Currency: "704"},
	}
}

func sampleRefundRequest() RefundRequest { return samplePreAuthRequest() }

func sampleBalanceRequest() BalanceInquiryRequest {
	return BalanceInquiryRequest{CardPresentData: CardPresentData{TerminalID: "00000042", CardToken: "tok_normal", EntryMode: "CHIP_PIN"}}
}

const stdMACHex = "0102030405060708"

func TestCreatePreAuth_sendsMTI0100WithDE25_06__MCN_603_AC1(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, _, _, _ := newTestService(mux)

	txn, err := svc.CreatePreAuth(context.Background(), samplePreAuthRequest(), "idem-preauth-1")

	require.NoError(t, err)
	require.Equal(t, "PREAUTH", txn.Type)
	require.Equal(t, "0100", mux.lastMTI)
	require.Equal(t, "06", mux.lastFields[25])
	require.Equal(t, "000000", mux.lastFields[3])
}

func TestCreateCompletion_sendsMTI0220ReferencingOriginalRRN__MCN_603_AC1(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)
	tranLog.rows = append(tranLog.rows, store.TranLogRow{RRN: "123456789012", Type: "PREAUTH", TerminalID: "00000042", MaskedPAN: "970436******4417"})

	txn, err := svc.CreateCompletion(context.Background(), "123456789012", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "idem-completion-1")

	require.NoError(t, err)
	require.Equal(t, "0220", mux.lastMTI)
	require.Equal(t, "123456789012", mux.lastFields[37])
	require.Equal(t, "123456789012", txn.OriginalRRN)
}

func TestMapResponseToTransaction_partialApprovalSetsApprovedAmountDistinctFromRequested__MCN_603_AC2(t *testing.T) {
	requested := Money{Amount: 10000, Currency: "704"}
	resp := map[int]string{39: "10", 4: "000000007500"}

	txn := mapResponseToTransaction("COMPLETION", requested, resp)

	require.Equal(t, "10", txn.ResponseCode)
	require.NotNil(t, txn.ApprovedAmount)
	require.Equal(t, int64(7500), txn.ApprovedAmount.Amount)
	require.NotEqual(t, requested.Amount, txn.ApprovedAmount.Amount)
}

func TestCreateRefund_sendsDE3_200000__MCN_603_AC1(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, _, _, _ := newTestService(mux)

	_, err := svc.CreateRefund(context.Background(), sampleRefundRequest(), "idem-refund-1")

	require.NoError(t, err)
	require.Equal(t, "200000", mux.lastFields[3])
}

func TestCreateBalanceInquiry_sendsDE3_310000AndParsesDE54__MCN_603_AC1(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex, 54: "704C000000012345"}}
	svc, _, _, _ := newTestService(mux)

	txn, err := svc.CreateBalanceInquiry(context.Background(), sampleBalanceRequest(), "idem-balance-1")

	require.NoError(t, err)
	require.Equal(t, "310000", mux.lastFields[3])
	require.NotNil(t, txn.Balance)
	require.Equal(t, int64(12345), txn.Balance.Amount)
}

func TestCreatePreAuth_unknownCardTokenReturnsError__MCN_603_AC1(t *testing.T) {
	mux := &fakeMux{}
	svc, _, _, _ := newTestService(mux)
	req := samplePreAuthRequest()
	req.CardToken = "tok_does_not_exist"

	_, err := svc.CreatePreAuth(context.Background(), req, "idem-preauth-2")

	require.ErrorIs(t, err, ErrUnknownCardToken)
}

func TestCreatePreAuth_replaysIdempotentRequest__MCN_603_AC1(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, _, _, hub := newTestService(mux)

	first, err := svc.CreatePreAuth(context.Background(), samplePreAuthRequest(), "same-key")
	require.NoError(t, err)
	mux.lastFields = nil

	second, err := svc.CreatePreAuth(context.Background(), samplePreAuthRequest(), "same-key")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Nil(t, mux.lastFields) // never resent
	require.Equal(t, 1, hub.broadcasts)
}

func TestCreatePreAuth_usesTerminalsMerchant__MCN_002(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 38: "654321", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)
	req := samplePreAuthRequest()
	req.TerminalID = "00000047"

	txn, err := svc.CreatePreAuth(context.Background(), req, "idem-preauth-merchant")

	require.NoError(t, err)
	require.Equal(t, "BANHMA000000001", mux.lastFields[42])
	require.Equal(t, "BANHMA000000001", tranLog.rows[0].MerchantID)
	require.Equal(t, "Tiệm bánh Mây", txn.MerchantName)
}

func TestCreateRefund_unknownTerminalSendsNothing__MCN_002(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, _, _, _ := newTestService(mux)
	req := sampleRefundRequest()
	req.TerminalID = "99999999"

	_, err := svc.CreateRefund(context.Background(), req, "idem-refund-unknown")

	require.ErrorIs(t, err, store.ErrUnknownTerminal)
	require.Nil(t, mux.lastFields)
}

func TestCreateCompletion_inheritsPreAuthsTerminalAndMerchant__MCN_002(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)
	tranLog.rows = append(tranLog.rows, store.TranLogRow{RRN: "626514000300", Type: "PREAUTH", TerminalID: "00000047", MaskedPAN: "970436******5540"})

	txn, err := svc.CreateCompletion(context.Background(), "626514000300", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "idem-completion-merchant")

	require.NoError(t, err)
	completion := tranLog.rows[len(tranLog.rows)-1]
	require.Equal(t, "00000047", completion.TerminalID)
	require.Equal(t, "BANHMA000000001", completion.MerchantID)
	require.Equal(t, "970436******5540", txn.MaskedPAN)
	require.Equal(t, "Tiệm bánh Mây", txn.MerchantName)
	_, hasDE42 := mux.lastFields[42]
	require.False(t, hasDE42, "0220 carries no DE 42")
}

func TestCreateCompletion_unknownPreAuthSendsNothing__MCN_002(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, _, _, _ := newTestService(mux)

	_, err := svc.CreateCompletion(context.Background(), "000000000000", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "idem-completion-unknown")

	require.ErrorIs(t, err, store.ErrNotFound)
	require.Nil(t, mux.lastFields)
}
