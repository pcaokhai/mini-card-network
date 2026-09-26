package advtxn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

type fakeMux struct {
	linkDown    bool
	response    map[int]string
	err         error
	lastMTI     string
	lastFields  map[int]string
	stanCounter int
	onSend      func() // runs inside Send, e.g. to cancel the caller's context mid-flight
}

func (f *fakeMux) NextSTAN() (string, bool) {
	if f.linkDown {
		return "", false
	}
	f.stanCounter++
	return "000001", true
}

func (f *fakeMux) IsSignedOn() bool { return !f.linkDown }

func (f *fakeMux) Send(_ context.Context, mti string, fields map[int]string) (map[int]string, error) {
	f.lastMTI = mti
	f.lastFields = fields
	if f.onSend != nil {
		f.onSend()
	}
	return f.response, f.err
}

type fakeTranLog struct {
	rows        []store.TranLogRow
	transitions []string
	failStatus  string // UpdateStatus to this status fails, leaving the row as it was
}

func (f *fakeTranLog) UpdateAmounts(_ context.Context, id int64, approved *int64, balance *store.Money) error {
	f.rows[id-1].ApprovedAmount = approved
	f.rows[id-1].Balance = balance
	return nil
}

func (f *fakeTranLog) ReleaseCompletion(_ context.Context, preAuthRRN, completionRRN string) error {
	for i := range f.rows {
		if f.rows[i].RRN == preAuthRRN && f.rows[i].CompletedBy == completionRRN {
			f.rows[i].CompletedBy = ""
		}
	}
	return nil
}

// InsertCompletion claims the pre-auth as store.TranLogRepository does, in one step.
func (f *fakeTranLog) InsertCompletion(ctx context.Context, row store.TranLogRow, preAuthRRN string) (int64, error) {
	for i := range f.rows {
		pre := &f.rows[i]
		if pre.RRN == preAuthRRN && pre.Type == tranTypePreAuth && pre.Status == statusApproved && pre.CompletedBy == "" {
			pre.CompletedBy = row.RRN
			return f.Insert(ctx, row)
		}
	}
	return 0, store.ErrNotCompletable
}

func (f *fakeTranLog) RecordStateTransition(_ context.Context, _ int64, from, to string) error {
	f.transitions = append(f.transitions, from+"->"+to)
	return nil
}

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

func (f *fakeTranLog) UpdateStatusFrom(ctx context.Context, id int64, from, status, responseCode, authCode string) error {
	if f.rows[id-1].Status != from {
		return store.ErrStateMoved
	}
	return f.UpdateStatus(ctx, id, status, responseCode, authCode)
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

// fakeIdempotency mirrors store.IdempotencyRepository's reserve/store/release contract,
// including its reservation tokens.
type fakeIdempotency struct {
	stored    map[string]store.StoredResponse
	storeCtx  context.Context // the context the last Store ran on
	hashes    map[string]string
	pending   map[string]bool
	released  []string
	rrns      map[string]string    // AttachRRN, by key+route
	reserved  map[string]time.Time // when each pending key was reserved
	tokens    map[string]string    // the token holding each pending key
	issued    int
	attachErr error
	// reclaimOnAttach makes another request reclaim the key just before AttachRRN, as when the
	// holder was slow, not dead.
	reclaimOnAttach bool
}

func (f *fakeIdempotency) init() {
	if f.hashes == nil {
		f.stored, f.hashes, f.pending = map[string]store.StoredResponse{}, map[string]string{}, map[string]bool{}
		f.rrns, f.reserved, f.tokens = map[string]string{}, map[string]time.Time{}, map[string]string{}
	}
}

func (f *fakeIdempotency) newToken(k string) string {
	f.issued++
	f.tokens[k] = fmt.Sprintf("token-%d", f.issued)
	return f.tokens[k]
}

// pendAt makes key a reservation made at reservedAt that sent rrn, as a crashed request leaves it.
func (f *fakeIdempotency) pendAt(key, route, hash, rrn string, reservedAt time.Time) {
	_, _, _ = f.Reserve(context.Background(), key, route, hash)
	k := key + route
	f.rrns[k], f.reserved[k] = rrn, reservedAt
}

func (f *fakeIdempotency) Reclaim(_ context.Context, key, route, hash string, staleAfter time.Duration) (string, error) {
	k := key + route
	if !f.pending[k] || f.hashes[k] != hash || f.rrns[k] != "" || time.Since(f.reserved[k]) < staleAfter {
		return "", nil
	}
	f.reserved[k] = time.Now()
	return f.newToken(k), nil
}

func (f *fakeIdempotency) AttachRRN(_ context.Context, key, route, token, rrn string) error {
	if f.attachErr != nil {
		return f.attachErr
	}
	k := key + route
	if f.reclaimOnAttach {
		f.newToken(k)
	}
	if f.tokens[k] != token {
		return store.ErrReservationLost
	}
	f.rrns[k] = rrn
	return nil
}

func (f *fakeIdempotency) Reserve(_ context.Context, key, route, hash string) (*store.StoredResponse, string, error) {
	f.init()
	k := key + route
	if h, ok := f.hashes[k]; ok {
		switch {
		case h != hash:
			return nil, "", store.ErrIdempotencyKeyMismatch
		case f.pending[k]:
			return nil, "", &store.InProgressError{RRN: f.rrns[k], ReservedAt: f.reserved[k]}
		}
		stored := f.stored[k]
		return &stored, "", nil
	}
	f.hashes[k], f.pending[k], f.reserved[k] = hash, true, time.Now()
	return nil, f.newToken(k), nil
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

func (f *fakeIdempotency) Release(_ context.Context, key, route, token string) error {
	k := key + route
	if f.pending[k] && f.tokens[k] == token {
		delete(f.pending, k)
		delete(f.hashes, k)
		f.released = append(f.released, key)
	}
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

var testZAK = hsm.StaticZAK(make([]byte, 16))

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

type fakeReversal struct {
	reasons    []string
	queued     []store.TranLogRow
	advices    []map[int]string // QueueAdvice's fields, keyed by DE
	adviceMTIs []string
}

func (f *fakeReversal) QueueAdvice(_ context.Context, _ int64, mti string, sent map[int]string) error {
	f.adviceMTIs = append(f.adviceMTIs, mti)
	f.advices = append(f.advices, sent)
	return nil
}

func (f *fakeReversal) Queue(_ context.Context, txn store.TranLogRow, reasonCode string) error {
	f.reasons = append(f.reasons, reasonCode)
	f.queued = append(f.queued, txn)
	return nil
}

func newTestService(mux *fakeMux) (*Service, *fakeTranLog, *fakeIdempotency, *fakeHub) {
	svc, tranLog, idem, hub, _ := newTestServiceWithReversal(mux)
	return svc, tranLog, idem, hub
}

func newTestServiceWithReversal(mux *fakeMux) (*Service, *fakeTranLog, *fakeIdempotency, *fakeHub, *fakeReversal) {
	tranLog := &fakeTranLog{}
	idem := &fakeIdempotency{}
	hub := &fakeHub{}
	reversal := &fakeReversal{}
	svc := NewService(mux, purchase.DefaultCardTokens(), testMerchants, tranLog, idem, hub, reversal, fakeHSM{}, testZAK, nil, testCalendar)
	return svc, tranLog, idem, hub, reversal
}

func samplePreAuthRequest() PreAuthRequest {
	return PreAuthRequest{
		CardPresentData: CardPresentData{TerminalID: "00000042", CardToken: testCardToken, EntryMode: "CHIP_PIN"},
		Amount:          Money{Amount: 10000, Currency: "704"},
	}
}

func sampleRefundRequest() RefundRequest { return samplePreAuthRequest() }

func sampleBalanceRequest() BalanceInquiryRequest {
	return BalanceInquiryRequest{CardPresentData: CardPresentData{TerminalID: "00000042", CardToken: testCardToken, EntryMode: "CHIP_PIN"}}
}

const (
	stdMACHex     = "0102030405060708"
	testCardToken = "tok_normal"
)

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
	tranLog.rows = append(tranLog.rows, store.TranLogRow{RRN: "123456789012", Type: tranTypePreAuth, Status: statusApproved, Amount: 10000, TerminalID: "00000042", MaskedPAN: "970436******4417"})

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
	tranLog.rows = append(tranLog.rows, store.TranLogRow{RRN: "626514000300", Type: tranTypePreAuth, Status: statusApproved, Amount: 10000, TerminalID: "00000047", MaskedPAN: "970436******5540"})

	txn, err := svc.CreateCompletion(context.Background(), "626514000300", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "idem-completion-merchant")

	require.NoError(t, err)
	completion := tranLog.rows[len(tranLog.rows)-1]
	require.Equal(t, "00000047", completion.TerminalID)
	require.Equal(t, "BANHMA000000001", completion.MerchantID)
	require.Equal(t, "970436******5540", txn.MaskedPAN)
	require.Equal(t, "Tiệm bánh Mây", txn.MerchantName)
	require.Equal(t, "00000047", mux.lastFields[41], "SF2: the issuer dedupes on the terminal, so the 0220 names it")
	require.Equal(t, "BANHMA000000001", mux.lastFields[42])
}

func TestCreateCompletion_unknownPreAuthSendsNothing__MCN_002(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, _, _, _ := newTestService(mux)

	_, err := svc.CreateCompletion(context.Background(), "000000000000", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "idem-completion-unknown")

	require.ErrorIs(t, err, store.ErrNotFound)
	require.Nil(t, mux.lastFields)
}

// sendOneOfEach runs every advanced flow once, with a PREAUTH row for the completion to reference.
func sendOneOfEach(t *testing.T, svc *Service, tranLog *fakeTranLog, key string) []Transaction {
	t.Helper()
	tranLog.rows = append(tranLog.rows, store.TranLogRow{RRN: "626514000300", Type: tranTypePreAuth, Status: "APPROVED", Amount: 10000, TerminalID: "00000042", MaskedPAN: "970436******4417", CardToken: testCardToken, POSEntryMode: "051"})
	ctx := context.Background()
	var txns []Transaction
	preAuth, err := svc.CreatePreAuth(ctx, samplePreAuthRequest(), key+"-preauth")
	require.NoError(t, err)
	completion, err := svc.CreateCompletion(ctx, "626514000300", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, key+"-completion")
	require.NoError(t, err)
	refund, err := svc.CreateRefund(ctx, sampleRefundRequest(), key+"-refund")
	require.NoError(t, err)
	balance, err := svc.CreateBalanceInquiry(ctx, sampleBalanceRequest(), key+"-balance")
	require.NoError(t, err)
	return append(txns, preAuth, completion, refund, balance)
}

func TestSend_everyPostSendFailureFollowsUpByType__POS_G4_POS_G9(t *testing.T) {
	failures := map[string]error{
		"timeout":           context.DeadlineExceeded,
		"request cancelled": context.Canceled,
		"broken pipe":       fmt.Errorf("write request: %w", syscall.EPIPE),
		"connection closed": io.EOF,
	}
	for name, sendErr := range failures {
		t.Run(name, func(t *testing.T) {
			svc, tranLog, _, _, reversal := newTestServiceWithReversal(&fakeMux{err: sendErr})

			txns := sendOneOfEach(t, svc, tranLog, name)

			// pre-auth, completion, refund, balance: the response reports each row's real status
			require.Equal(t, []string{statusReversalPending, statusTimedOut, statusReversalPending, statusTimedOut},
				[]string{txns[0].Status, txns[1].Status, txns[2].Status, txns[3].Status})
			require.Equal(t, []string{"68", "68"}, reversal.reasons, "a pre-auth and a refund are reversed")
			require.Equal(t, []string{tranTypePreAuth, tranTypeRefund}, []string{reversal.queued[0].Type, reversal.queued[1].Type})
			for _, queued := range reversal.queued {
				require.Equal(t, statusTimedOut, queued.Status, "queued from the state the row holds")
				require.NotEmpty(t, queued.CardToken, "the SAF worker rebuilds DE 2 from the card token")
			}
			require.Equal(t, []string{"0220"}, reversal.adviceMTIs, "a completion is an advice: repeated, never reversed (docs/03 §7.4)")
			require.Equal(t, "000001", reversal.advices[0][11])
			require.Equal(t, "00000042", reversal.advices[0][41], "SF2: every 0221 repeat names the terminal the issuer dedupes on")
			require.Equal(t, "GOCPHO000000001", reversal.advices[0][42])
			for _, row := range tranLog.rows[1:] {
				require.Equal(t, statusTimedOut, row.Status, "the row never stays SENT")
			}
		})
	}
}

func TestSend_linkDownDeclinesRc91WithoutSending__POS_G4(t *testing.T) {
	mux := &fakeMux{linkDown: true}
	svc, tranLog, _, hub, reversal := newTestServiceWithReversal(mux)

	txns := sendOneOfEach(t, svc, tranLog, "link-down")

	for i, txn := range txns {
		require.Equal(t, statusDeclined, txn.Status, txn.Type)
		require.Equal(t, "91", txn.ResponseCode, txn.Type)
		require.Equal(t, statusDeclined, tranLog.rows[i+1].Status)
		require.Equal(t, "91", tranLog.rows[i+1].ResponseCode)
		require.Nil(t, tranLog.rows[i+1].SentAt, "never sent")
	}
	require.Nil(t, mux.lastFields)
	require.Empty(t, reversal.reasons)
	require.Equal(t, 4, hub.broadcasts)
}

func TestSend_linkDroppedBeforeWriteDeclinesRc91WithoutReversal__POS_G4(t *testing.T) {
	svc, tranLog, _, _, reversal := newTestServiceWithReversal(&fakeMux{err: isonet.ErrNotSignedOn})

	txn, err := svc.CreateRefund(context.Background(), sampleRefundRequest(), "idem-refund-dropped")

	require.NoError(t, err)
	require.Equal(t, statusDeclined, txn.Status)
	require.Equal(t, "91", txn.ResponseCode)
	require.Equal(t, statusDeclined, tranLog.rows[0].Status)
	require.Len(t, tranLog.rows, 1, "the row sendAndRecord logged is updated, never logged twice")
	require.Empty(t, reversal.reasons)
}

func TestSend_badIncomingMACDeclinesRc96AndQueuesReasonSixReversal__POS_G5(t *testing.T) {
	for name, resp := range map[string]map[int]string{
		"wrong MAC":   {39: "00", 38: "123456", 64: badMACHex},
		"missing MAC": {39: "00", 38: "123456"},
	} {
		t.Run(name, func(t *testing.T) {
			svc, tranLog, _, _, reversal := newTestServiceWithReversal(&fakeMux{response: resp})

			txn, err := svc.CreatePreAuth(context.Background(), samplePreAuthRequest(), "idem-mac")

			require.NoError(t, err)
			require.Equal(t, statusDeclined, txn.Status)
			require.Equal(t, "96", txn.ResponseCode)
			require.Equal(t, []string{"06"}, reversal.reasons)
			require.Equal(t, statusDeclined, reversal.queued[0].Status)
			require.Equal(t, "96", tranLog.rows[0].ResponseCode)
		})
	}
}

func TestSend_badMACOnACompletionRepeatsTheAdvice__POS_G5(t *testing.T) {
	svc, tranLog, _, _, reversal := newTestServiceWithReversal(&fakeMux{response: map[int]string{39: "00", 64: badMACHex}})
	tranLog.rows = append(tranLog.rows, store.TranLogRow{RRN: "626514000300", Type: tranTypePreAuth, Status: statusApproved, Amount: 10000, TerminalID: "00000042", CardToken: testCardToken})

	txn, err := svc.CreateCompletion(context.Background(), "626514000300", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "idem-completion-mac")

	require.NoError(t, err)
	require.Equal(t, statusTimedOut, txn.Status, "a 0230 that fails its MAC proves nothing: the advice is repeated")
	require.Empty(t, reversal.reasons)
	require.Equal(t, []string{"0220"}, reversal.adviceMTIs)
}

func TestSend_badMACOnABalanceInquiryQueuesNothing__POS_G5(t *testing.T) {
	svc, _, _, _, reversal := newTestServiceWithReversal(&fakeMux{response: map[int]string{39: "00", 64: badMACHex}})

	txn, err := svc.CreateBalanceInquiry(context.Background(), sampleBalanceRequest(), "idem-balance-mac")

	require.NoError(t, err)
	require.Equal(t, statusDeclined, txn.Status)
	require.Equal(t, "96", txn.ResponseCode)
	require.Empty(t, reversal.reasons, "a balance inquiry holds no money")
	require.Empty(t, reversal.adviceMTIs)
}

func TestSend_callerGivingUpMidSendStillStoresTheOutcome__POS_G4(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc, _, idem, _ := newTestService(&fakeMux{response: map[int]string{39: "00", 64: stdMACHex}, onSend: cancel})

	txn, err := svc.CreateRefund(ctx, sampleRefundRequest(), "idem-gave-up")

	require.NoError(t, err)
	require.Equal(t, statusApproved, txn.Status)
	require.Len(t, idem.stored, 1, "a retry with the same key must replay, never resend")
	_, hasDeadline := idem.storeCtx.Deadline()
	require.True(t, hasDeadline, "work after the send is detached from the caller but still bounded")
}

func TestSend_reversalIsQueuedFromTheRowsStateWhenTheUpdateFails__POS_G4(t *testing.T) {
	for name, tc := range map[string]struct {
		fail string
		want string
	}{
		"status update fails":  {statusTimedOut, statusSent},
		"status update worked": {"", statusTimedOut},
	} {
		t.Run(name, func(t *testing.T) {
			svc, tranLog, _, _, reversal := newTestServiceWithReversal(&fakeMux{err: context.DeadlineExceeded})
			tranLog.failStatus = tc.fail

			_, err := svc.CreatePreAuth(context.Background(), samplePreAuthRequest(), "idem-status")

			require.NoError(t, err, "a failure after the send is answered from tran_log (S1)")
			require.Equal(t, []string{"68"}, reversal.reasons)
			require.Equal(t, tc.want, reversal.queued[0].Status, "Queue refuses any state but the one the row holds")
		})
	}
}

func TestSend_recordsWhatTheJourneyAndA0420Need__JRN_G1(t *testing.T) {
	svc, tranLog, _, _ := newTestService(&fakeMux{response: map[int]string{39: "00", 38: "123456", 64: stdMACHex}})

	_, err := svc.CreatePreAuth(context.Background(), samplePreAuthRequest(), "idem-journey")

	require.NoError(t, err)
	row := tranLog.rows[0]
	require.NotNil(t, row.SentAt)
	require.Equal(t, "000001", row.NetworkSTAN)
	require.Equal(t, "000000", row.ProcessingCode)
	require.Equal(t, "051", row.POSEntryMode)
	require.Equal(t, testCardToken, row.CardToken)
	require.Equal(t, "0100", row.MTI)
	require.Equal(t, []string{"CREATED->SENT", "SENT->APPROVED"}, tranLog.transitions)
}

func TestResponseMTI(t *testing.T) {
	require.Equal(t, "0110", responseMTI("0100"))
	require.Equal(t, "0210", responseMTI("0200"))
	require.Equal(t, "0230", responseMTI("0220"))
}

// badMACHex never matches the fake HSM's MAC.
const badMACHex = "FFFFFFFFFFFFFFFF"

func TestCreatePreAuth_replayNeverAllocatesASTAN__POS_G3(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, _, _, _ := newTestService(mux)
	_, err := svc.CreatePreAuth(context.Background(), samplePreAuthRequest(), "key-replay")
	require.NoError(t, err)

	_, err = svc.CreatePreAuth(context.Background(), samplePreAuthRequest(), "key-replay")

	require.NoError(t, err)
	require.Equal(t, 1, mux.stanCounter)
}

func TestCreateRefund_sameKeyDifferentRequestIsAMismatch__POS_G3(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, _, _, _ := newTestService(mux)
	_, err := svc.CreateRefund(context.Background(), sampleRefundRequest(), "key-refund")
	require.NoError(t, err)

	other := sampleRefundRequest()
	other.Amount.Amount = 1
	_, err = svc.CreateRefund(context.Background(), other, "key-refund")

	require.ErrorIs(t, err, store.ErrIdempotencyKeyMismatch)
	require.Equal(t, 1, mux.stanCounter)
}

func TestCreateBalanceInquiry_amountCarriesTheCardCurrency__POS_G7(t *testing.T) {
	svc, tranLog, _, _ := newTestService(&fakeMux{response: map[int]string{39: "00", 64: stdMACHex, 54: "704C000000012345"}})

	txn, err := svc.CreateBalanceInquiry(context.Background(), sampleBalanceRequest(), "key-balance")

	require.NoError(t, err)
	require.Equal(t, Money{Amount: 0, Currency: "704"}, txn.Amount)
	require.Equal(t, "704", tranLog.rows[0].Currency)
	require.Equal(t, &store.Money{Amount: 12345, Currency: "704"}, tranLog.rows[0].Balance, "JRN-G3")
}

func TestCreateCompletion_requiresAnApprovedPreAuth__POS_G13(t *testing.T) {
	for name, original := range map[string]store.TranLogRow{
		"a purchase":           {RRN: "626514000301", Type: "PURCHASE", Status: statusApproved, TerminalID: "00000042"},
		"a declined pre-auth":  {RRN: "626514000301", Type: tranTypePreAuth, Status: statusDeclined, TerminalID: "00000042"},
		"a reversing pre-auth": {RRN: "626514000301", Type: tranTypePreAuth, Status: statusReversalPending, TerminalID: "00000042"},
	} {
		t.Run(name, func(t *testing.T) {
			mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
			svc, tranLog, idem, _ := newTestService(mux)
			tranLog.rows = append(tranLog.rows, original)

			_, err := svc.CreateCompletion(context.Background(), "626514000301", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "key-completion")

			require.ErrorIs(t, err, ErrNotCompletable)
			require.Nil(t, mux.lastFields)
			require.Equal(t, []string{"key-completion"}, idem.released)
		})
	}
}

func TestCreateCompletion_persistsTheOriginalRRNAndApprovedAmount__JRN_G3(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "10", 4: "000000004000", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)
	tranLog.rows = append(tranLog.rows, store.TranLogRow{RRN: "626514000302", Type: tranTypePreAuth, Status: statusApproved, Amount: 10000, TerminalID: "00000042", CardToken: testCardToken})

	txn, err := svc.CreateCompletion(tracedContext(), "626514000302", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "key-completion-10")

	require.NoError(t, err)
	row := tranLog.rows[1]
	require.Equal(t, "626514000302", row.OriginalRRN)
	require.Equal(t, int64(4000), *row.ApprovedAmount)
	require.Equal(t, testTraceID, row.TraceID)
	require.Equal(t, testTraceID, txn.TraceID, "POS-G8")
	require.NotNil(t, txn.ResponseLabel, "POS-G8")
	require.NotNil(t, txn.LatencyMs, "POS-G8")
}

func TestSend_responseWithoutRCIsDeclinedFormatError__OVW_G11(t *testing.T) {
	svc, tranLog, _, _ := newTestService(&fakeMux{response: map[int]string{64: stdMACHex}})

	txn, err := svc.CreateRefund(context.Background(), sampleRefundRequest(), "key-no-rc")

	require.NoError(t, err)
	require.Equal(t, statusDeclined, txn.Status)
	require.Equal(t, "30", txn.ResponseCode)
	require.Equal(t, "30", tranLog.rows[0].ResponseCode)
}

const testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

func tracedContext() context.Context {
	traceID, _ := trace.TraceIDFromHex(testTraceID)
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	return trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID}))
}

func approvedPreAuth(rrn string) store.TranLogRow {
	return store.TranLogRow{RRN: rrn, Type: tranTypePreAuth, Status: statusApproved, Amount: 10000, Currency: "704", TerminalID: "00000042", CardToken: testCardToken}
}

func TestCreateCompletion_aPreAuthIsCompletedOnce__S2(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)
	tranLog.rows = append(tranLog.rows, approvedPreAuth("626514000950"))
	completion := CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}

	_, err := svc.CreateCompletion(context.Background(), "626514000950", completion, "key-complete-1")
	require.NoError(t, err)
	mux.lastFields = nil
	_, err = svc.CreateCompletion(context.Background(), "626514000950", completion, "key-complete-2")

	require.ErrorIs(t, err, ErrNotCompletable)
	require.Nil(t, mux.lastFields, "the second completion is never sent")
	require.Len(t, tranLog.rows, 2, "and never logged")
}

func TestCreateCompletion_cannotExceedTheHold__S2(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)
	tranLog.rows = append(tranLog.rows, approvedPreAuth("626514000951"))

	_, err := svc.CreateCompletion(context.Background(), "626514000951", CompletionRequest{Amount: Money{Amount: 10001, Currency: "704"}}, "key-over-hold")

	require.ErrorIs(t, err, ErrExceedsHold)
	require.Nil(t, mux.lastFields)
}

func TestCreateRefund_aRetryOfAStaleKeyAnswersFromTheTransaction__S1(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, tranLog, idem, _ := newTestService(mux)
	tranLog.rows = append(tranLog.rows, store.TranLogRow{RRN: "626514000952", Type: tranTypeRefund, Status: statusApproved, ResponseCode: "00", Amount: 10000, Currency: "704"})
	idem.pendAt("key-crashed", routeRefund, hashRequest(sampleRefundRequest()), "626514000952", time.Now().Add(-2*time.Minute))

	txn, err := svc.CreateRefund(context.Background(), sampleRefundRequest(), "key-crashed")

	require.NoError(t, err)
	require.Equal(t, statusApproved, txn.Status)
	require.Equal(t, "626514000952", txn.RRN)
	require.Zero(t, mux.stanCounter, "never resent")
}

func TestCreateCompletion_aPreSendFailureReleasesTheClaim__S2(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)
	tranLog.rows = append(tranLog.rows, approvedPreAuth("626514000970"))
	tranLog.failStatus = statusSent
	completion := CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}

	_, err := svc.CreateCompletion(context.Background(), "626514000970", completion, "key-fails-before-send")
	require.Error(t, err)
	require.Nil(t, mux.lastFields, "nothing was sent")
	require.Empty(t, tranLog.rows[0].CompletedBy, "the hold is free again")

	tranLog.failStatus = ""
	txn, err := svc.CreateCompletion(context.Background(), "626514000970", completion, "key-fails-before-send")
	require.NoError(t, err, "the retry completes instead of a 409 forever")
	require.Equal(t, statusApproved, txn.Status)
}

func TestCreateCompletion_aDeclineFreesThePreAuth__S2(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "05", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)
	tranLog.rows = append(tranLog.rows, approvedPreAuth("626514000971"))

	txn, err := svc.CreateCompletion(context.Background(), "626514000971", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "key-declined")

	require.NoError(t, err)
	require.Equal(t, statusDeclined, txn.Status)
	require.Empty(t, tranLog.rows[0].CompletedBy, "a declined completion consumed nothing")
}

func TestCreateCompletion_anUnknownOutcomeKeepsTheClaim__S2(t *testing.T) {
	svc, tranLog, _, _ := newTestService(&fakeMux{err: context.DeadlineExceeded})
	tranLog.rows = append(tranLog.rows, approvedPreAuth("626514000972"))

	_, err := svc.CreateCompletion(context.Background(), "626514000972", CompletionRequest{Amount: Money{Amount: 5000, Currency: "704"}}, "key-unknown")

	require.NoError(t, err)
	require.NotEmpty(t, tranLog.rows[0].CompletedBy, "the 0220 is repeated through SAF, so the hold stays taken")
}

func TestSend_aPreSendFailureLeavesNoSentRow__N2(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, tranLog, idem, _ := newTestService(mux)
	idem.attachErr = errors.New("db down")

	_, err := svc.CreateRefund(context.Background(), sampleRefundRequest(), "key-attach-fails")

	require.Error(t, err)
	require.Nil(t, mux.lastFields, "nothing was sent")
	require.NotEqual(t, statusSent, tranLog.rows[0].Status, "a SENT row would be swept into a 0420 for a request that never left")
}

func TestSend_aLateAnswerNeverOverwritesASweptRow__N1(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)
	mux.onSend = func() { tranLog.rows[0].Status = statusTimedOut } // the sweeper gave up on the stalled request

	txn, err := svc.CreateRefund(context.Background(), sampleRefundRequest(), "key-swept")

	require.NoError(t, err)
	require.Equal(t, statusTimedOut, tranLog.rows[0].Status, "the sweep's reversal stands")
	require.Equal(t, statusTimedOut, txn.Status)
}

// fixedCalendar is a BusinessCalendar whose business date never moves.
type fixedCalendar struct{ date time.Time }

func (c fixedCalendar) Current(time.Time) time.Time    { return c.date }
func (c fixedCalendar) Previous(d time.Time) time.Time { return d.AddDate(0, 0, -1) }
func (c fixedCalendar) OpenedAt(d time.Time) time.Time { return d }

// testCalendar's business date is deliberately not today's UTC date.
var testCalendar = fixedCalendar{date: time.Date(2031, 12, 31, 0, 0, 0, 0, time.UTC)}

func TestSend_carriesTheBusinessDateInDE15AndTheRow__OVW_G7(t *testing.T) {
	mux := &fakeMux{response: map[int]string{39: "00", 64: stdMACHex}}
	svc, tranLog, _, _ := newTestService(mux)

	txn, err := svc.CreatePreAuth(context.Background(), samplePreAuthRequest(), "idem-business-date")

	require.NoError(t, err)
	require.Equal(t, "1231", mux.lastFields[15], "DE 15 is mandatory in the 0100 (docs/03 §3)")
	require.Equal(t, testCalendar.date, tranLog.rows[0].BusinessDate)
	require.Equal(t, "2031-12-31", txn.BusinessDate)
}
