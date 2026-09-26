// Package advtxn builds and sends the advanced transaction family (pre-authorization,
// completion, refund, balance inquiry) through the same mux/codec/MAC pipeline
// purchase.Service established for the purchase flow (MCN-303). Kept as a sibling package
// rather than folded into purchase.Service per docs/plans/MCN-603.md Ruling 2: these four
// flows' request shapes genuinely differ (completion has no card-present data; balance
// inquiry has no amount), so one shared struct would need optional fields for every flow.
//
// Ruling 1 (docs/plans/MCN-603.md): this sprint proves these endpoints against
// internal/chaos/fakeissuer, not a real issuer-side hold/completion chain (that's MCN-601,
// Sprint 8).
package advtxn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mcn/gateway-go/internal/bizdate"
	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/iso8583"
	"github.com/mcn/gateway-go/internal/journey"
	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	acquirerID     = "970499" // docs/03 §3, same lab acquirer ID purchase.Service uses
	requestTimeout = 30 * time.Second

	rcPartialApproval = "10" // docs/03 C1: partial approval, DE 4 carries the approved amount
	rcLinkDown        = "91" // docs/03 §8: issuer or switch inoperative
	rcMacFailure      = "96" // docs/03 §11: MAC verification failed

	reasonTimeout    = "68" // docs/03 §7.3 DE 39 of the 0420: no response / timeout
	reasonMacFailure = "06" // docs/03 §7.3 DE 39 of the 0420: error (a bad incoming MAC)

	statusCreated         = "CREATED"
	statusSent            = "SENT"
	statusApproved        = "APPROVED"
	statusDeclined        = "DECLINED"
	statusTimedOut        = "TIMED_OUT"
	statusReversalPending = "REVERSAL_PENDING"

	// noSTAN names a request that never got a network STAN because the link was down, as
	// purchase.Service does (docs/03 §5 RRN format).
	noSTAN = "000000"

	eventCreated = "transaction.created"
)

// ErrNotCompletable means a completion names a transaction that is not an approved
// pre-authorization, or one already completed (POS-G13).
var ErrNotCompletable = store.ErrNotCompletable

// ErrExceedsHold means a completion asks for more than its pre-authorization held.
var ErrExceedsHold = errors.New("completion amount exceeds the pre-authorized amount")

// Money mirrors contracts/openapi.yaml's Money schema; reused directly from purchase per DRY
// (docs/plans/MCN-603.md Task 1) since the shape is identical.
type Money = purchase.Money

// CardPresentData mirrors contracts/openapi.yaml's CardPresentData schema (card token + entry
// mode, no amount) - the shape balance inquiry needs and pre-auth/refund embed.
type CardPresentData struct {
	TerminalID        string `json:"terminalId"`
	CardToken         string `json:"cardToken"`
	EntryMode         string `json:"entryMode"`
	EncryptedPinBlock string `json:"encryptedPinBlock,omitempty"`
	EMVData           string `json:"emvData,omitempty"`
}

// PreAuthRequest mirrors contracts/openapi.yaml's PurchaseRequest schema (reused for pre-auth
// and refund, which both need card-present data + amount).
type PreAuthRequest struct {
	CardPresentData
	Amount Money `json:"amount"`
}

// RefundRequest has the same shape as PreAuthRequest per contracts/openapi.yaml.
type RefundRequest = PreAuthRequest

// CompletionRequest mirrors contracts/openapi.yaml's inline completion request body: just the
// completion amount, referencing the pre-auth's RRN via the URL path.
type CompletionRequest struct {
	Amount Money `json:"amount"`
}

// BalanceInquiryRequest mirrors contracts/openapi.yaml's CardPresentData schema: no amount.
type BalanceInquiryRequest struct {
	CardPresentData
}

// Transaction mirrors contracts/openapi.yaml's Transaction schema's advanced-transaction
// fields (approvedAmount, balance, originalRrn).
type Transaction struct {
	RRN            string    `json:"rrn"`
	STAN           string    `json:"stan,omitempty"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	ResponseCode   string    `json:"responseCode,omitempty"`
	Amount         Money     `json:"amount"`
	ApprovedAmount *Money    `json:"approvedAmount,omitempty"`
	Balance        *Money    `json:"balance,omitempty"`
	MaskedPAN      string    `json:"maskedPan,omitempty"`
	TerminalID     string    `json:"terminalId,omitempty"`
	MerchantName   string    `json:"merchantName,omitempty"`
	AuthCode       string    `json:"authCode,omitempty"`
	OriginalRRN    string    `json:"originalRrn,omitempty"`
	BusinessDate   string    `json:"businessDate"`
	CreatedAt      time.Time `json:"createdAt"`
	ResponseLabel  *string   `json:"responseLabel"`
	LatencyMs      *int      `json:"latencyMs"`
	TraceID        string    `json:"traceId,omitempty"`
}

// MuxSender sends a request on the acquirer's live issuer connection; *isonet.Supervisor
// satisfies it (same port purchase.Service uses).
type MuxSender interface {
	NextSTAN() (stan string, ok bool)
	Send(ctx context.Context, mti string, fields map[int]string) (map[int]string, error)
	IsSignedOn() bool
}

// TranLogPort persists tran_log rows and their state history; *store.TranLogRepository
// satisfies it.
type TranLogPort interface {
	Insert(ctx context.Context, row store.TranLogRow) (int64, error)
	Get(ctx context.Context, rrn string) (store.TranLogRow, error)
	UpdateStatus(ctx context.Context, id int64, status, responseCode, authCode string) error
	UpdateStatusFrom(ctx context.Context, id int64, from, status, responseCode, authCode string) error
	UpdateAmounts(ctx context.Context, id int64, approvedAmount *int64, balance *store.Money) error
	InsertCompletion(ctx context.Context, completion store.TranLogRow, preAuthRRN string) (int64, error)
	ReleaseCompletion(ctx context.Context, preAuthRRN, completionRRN string) error
	RecordStateTransition(ctx context.Context, id int64, fromStatus, toStatus string) error
}

// HubPort broadcasts a transaction event; internal/ws.Hub (extended for purchase.Service)
// satisfies it.
type HubPort interface {
	BroadcastTransaction(eventType string, txn Transaction)
}

// SAFQueuer stores what an unknown outcome needs delivered later; *saf.ReversalQueuer satisfies
// it. Queue reverses a request with a 0420; QueueAdvice repeats an advice (a 0220) until its x30.
type SAFQueuer interface {
	purchase.ReversalQueuer
	QueueAdvice(ctx context.Context, tranID int64, mti string, sent map[int]string) error
}

// Service builds and sends advanced (pre-auth/completion/refund/balance) transactions.
type Service struct {
	mux         MuxSender
	cardTokens  *purchase.CardTokenRegistry
	merchants   purchase.MerchantResolver
	tranLog     TranLogPort
	idempotency purchase.IdempotencyPort
	hub         HubPort
	saf         SAFQueuer
	hsm         hsm.Module
	zak         hsm.ZAKSource // read per message (SEC-G10)
	mac         purchase.MACVerifier
	calendar    bizdate.Calendar
}

// NewService builds a Service, reusing the same dependency shapes purchase.NewService takes -
// Ruling 2: a sibling service, not a re-derivation of purchase.Service's already-proven wiring.
// safQueuer and keyStore are the same SAF queue and dual-key MAC lookup purchases use; calendar
// is the acquirer's business date (ADR-007).
func NewService(mux MuxSender, cardTokens *purchase.CardTokenRegistry, merchants purchase.MerchantResolver, tranLog TranLogPort, idempotency purchase.IdempotencyPort, hub HubPort, safQueuer SAFQueuer, hsmModule hsm.Module, zak hsm.ZAKSource, keyStore purchase.RetiredKeyFinder, calendar bizdate.Calendar) *Service {
	return &Service{
		mux: mux, cardTokens: cardTokens, merchants: merchants, tranLog: tranLog, idempotency: idempotency, hub: hub,
		saf: safQueuer, hsm: hsmModule, zak: zak, mac: purchase.NewMACVerifier(hsmModule, zak, keyStore), calendar: calendar,
	}
}

// sendParams carries what each flow-specific Create* method fills in before calling send.
type sendParams struct {
	mti          string
	txnType      string
	route        string
	fields       map[int]string
	rrn, stan    string
	linkUp       bool
	sentAt       time.Time // the moment DE 7 was built from
	maskedPAN    string
	cardToken    string
	posEntryMode string
	terminalID   string
	requestedAmt Money
	originalRRN  string
	businessDate time.Time // set by send: the business date open when DE 7 was built
}

// nextSTAN allocates the network STAN. linkUp is false when the issuer link can't carry the
// request; the flow then records a DECLINED RC 91 under noSTAN without sending (MCN-303-AC4).
func (s *Service) nextSTAN() (stan string, linkUp bool) {
	stan, ok := s.mux.NextSTAN()
	if !ok || !s.mux.IsSignedOn() {
		return noSTAN, false
	}
	return stan, true
}

// finalizeFields attributes the message to the terminal's merchant, then MACs it: DE 42 is
// rewritten only for flows whose message carries it (completion's 0220 has none), and before the
// MAC so the MAC covers it.
func (s *Service) finalizeFields(ctx context.Context, p sendParams) (store.Merchant, error) {
	// DE 15, mandatory in the 0100, 0200 and 0220 (docs/03 §3), carries the business date.
	p.fields[15] = bizdate.MMDD(p.businessDate)
	merchant, err := s.merchants.Merchant(ctx, p.terminalID)
	if err != nil {
		return store.Merchant{}, fmt.Errorf("resolve merchant for terminal %s: %w", p.terminalID, err)
	}
	if _, ok := p.fields[42]; ok {
		p.fields[42] = merchant.MID
	}
	if err := attachMAC(s.hsm, s.zak.ActiveZAK(), p.mti, p.fields); err != nil {
		return store.Merchant{}, fmt.Errorf("compute outgoing MAC: %w", err)
	}
	return merchant, nil
}

// idempotent runs create once per (key, route), before any STAN is allocated, so a replay never
// consumes one (POS-G3).
func (s *Service) idempotent(ctx context.Context, route, key string, req any, create func(ctx context.Context) (Transaction, error)) (Transaction, error) {
	return purchase.Idempotent(ctx, s.idempotency, key, route, hashRequest(req), http.StatusCreated, create, s.fromLog)
}

// fromLog answers a request whose response was never recorded from its tran_log row; the answer
// is final once the row has left CREATED/SENT.
func (s *Service) fromLog(ctx context.Context, rrn string) (Transaction, bool, error) {
	row, err := s.tranLog.Get(ctx, rrn)
	if err != nil {
		return Transaction{}, false, err
	}
	txn := Transaction{
		RRN: row.RRN, STAN: row.NetworkSTAN, Type: row.Type, Status: row.Status, ResponseCode: row.ResponseCode,
		Amount: Money{Amount: row.Amount, Currency: row.Currency}, MaskedPAN: row.MaskedPAN, TerminalID: row.TerminalID,
		MerchantName: row.MerchantName, AuthCode: row.AuthCode, OriginalRRN: row.OriginalRRN,
		BusinessDate: bizdate.Format(row.BusinessDate), CreatedAt: row.CreatedAt,
		ResponseLabel: purchase.ResponseLabel(row.ResponseCode), LatencyMs: journey.LatencyMs(row), TraceID: row.TraceID,
	}
	if row.ApprovedAmount != nil {
		txn.ApprovedAmount = &Money{Amount: *row.ApprovedAmount, Currency: row.Currency}
	}
	if row.Balance != nil {
		txn.Balance = &Money{Amount: row.Balance.Amount, Currency: row.Balance.Currency}
	}
	return txn, row.Status != statusCreated && row.Status != statusSent, nil
}

// send MACs and sends p.fields, persists the outcome and broadcasts it - the one place every
// flow's send path runs (Global Constraints: every outgoing message is MAC'd, no new path
// bypasses it).
func (s *Service) send(ctx context.Context, p sendParams) (Transaction, error) {
	// The business date the request is sent in, which it keeps even if answered after cutover.
	p.businessDate = s.calendar.Current(p.sentAt)
	merchant, err := s.finalizeFields(ctx, p)
	if err != nil {
		return Transaction{}, err
	}

	if !p.linkUp {
		txn, err := s.recordLinkDown(ctx, p, merchant)
		if err != nil {
			return Transaction{}, err
		}
		s.hub.BroadcastTransaction(eventCreated, txn)
		return txn, nil
	}
	txn, err := s.sendAndRecord(ctx, p, merchant)
	if err != nil {
		return Transaction{}, err
	}
	s.hub.BroadcastTransaction(eventCreated, txn)
	return txn, nil
}

// sendAndRecord logs the row as SENT, sends it, and records the outcome. Every failure after the
// write is an unknown outcome: the row goes TIMED_OUT and a 0420 is queued, never a resend (root
// CLAUDE.md §6.4); a bad incoming MAC is DECLINED RC 96 with a reason-06 reversal (MCN-502-AC3).
func (s *Service) sendAndRecord(ctx context.Context, p sendParams, merchant store.Merchant) (Transaction, error) {
	row := store.TranLogRow{
		RRN: p.rrn, Type: p.txnType, Status: statusCreated, Amount: p.requestedAmt.Amount, Currency: p.requestedAmt.Currency,
		MaskedPAN: p.maskedPAN, TerminalID: p.terminalID, MerchantID: merchant.MID, NetworkSTAN: p.stan, MTI: p.mti,
		ProcessingCode: p.fields[3], POSEntryMode: p.posEntryMode, SentAt: &p.sentAt, CardToken: p.cardToken, BusinessDate: p.businessDate,
		OriginalRRN: p.originalRRN, TraceID: obs.TraceID(ctx),
	}
	id, err := s.insert(ctx, p, row)
	if err != nil {
		return Transaction{}, err
	}
	if err := s.markSending(ctx, id, p); err != nil {
		// Nothing was sent: a completion's hold is free again, as its idempotency key is.
		return Transaction{}, errors.Join(err, s.releaseClaim(ctx, p))
	}

	sendCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	sendStart := time.Now()
	resp, sendErr := s.mux.Send(sendCtx, p.mti, p.fields)
	latencyMs := int(time.Since(sendStart).Milliseconds())
	// The request may already be at the issuer: the outcome must be recorded whatever happened to
	// the caller's request.
	ctx, cancelPersist := purchase.Detach(ctx)
	defer cancelPersist()

	txn, macFailed := s.outcome(ctx, p, resp, sendErr)
	txn = s.describe(ctx, txn, p, merchant)
	if sendErr == nil {
		txn.LatencyMs = &latencyMs
	}
	row.ID = id
	finalized, err := s.finalize(ctx, p, row, txn, macFailed)
	return finalized, afterSend(err)
}

// recordAmounts stores what the response reported beyond its RC (JRN-G3).
func (s *Service) recordAmounts(ctx context.Context, id int64, txn Transaction) error {
	if txn.ApprovedAmount == nil && txn.Balance == nil {
		return nil
	}
	var approved *int64
	if txn.ApprovedAmount != nil {
		approved = &txn.ApprovedAmount.Amount
	}
	if err := s.tranLog.UpdateAmounts(ctx, id, approved, (*store.Money)(txn.Balance)); err != nil {
		return fmt.Errorf("update tran_log amounts: %w", err)
	}
	return nil
}

// afterSend marks err as raised once the request may be at the issuer, so its idempotency key is
// never released for a resend (nil stays nil).
func afterSend(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", purchase.ErrAfterSend, err)
}

// markSending records the RRN on the idempotency key and then moves row id to SENT, the last
// step before the send: a row that fails before it stays CREATED, so the orphan sweeper never
// reverses (or repeats) a request that never left.
func (s *Service) markSending(ctx context.Context, id int64, p sendParams) error {
	if err := purchase.AttachRRN(ctx, p.rrn); err != nil {
		return err
	}
	return s.transition(ctx, id, statusCreated, statusSent, "", "")
}

// releaseClaim frees a completion's claim on its pre-auth when the completion consumed nothing:
// it was never sent, or the issuer declined it. An unknown outcome keeps the claim, since its
// 0220 is repeated through SAF.
func (s *Service) releaseClaim(ctx context.Context, p sendParams) error {
	if p.txnType != tranTypeCompletion {
		return nil
	}
	if err := s.tranLog.ReleaseCompletion(ctx, p.originalRRN, p.rrn); err != nil {
		return fmt.Errorf("release %s's claim on %s: %w", p.rrn, p.originalRRN, err)
	}
	return nil
}

// insert logs row about to be sent. A completion also claims its pre-authorization in the same
// DB transaction, so a pre-auth is completed at most once (#117 review S2).
func (s *Service) insert(ctx context.Context, p sendParams, row store.TranLogRow) (int64, error) {
	if p.txnType != tranTypeCompletion {
		id, err := s.tranLog.Insert(ctx, row)
		if err != nil {
			return 0, fmt.Errorf("insert tran_log: %w", err)
		}
		return id, nil
	}
	id, err := s.tranLog.InsertCompletion(ctx, row, p.originalRRN)
	if err != nil {
		return 0, fmt.Errorf("insert completion of %s: %w", p.originalRRN, err)
	}
	return id, nil
}

// outcome maps mux.Send's result onto a Transaction, verifying the response's MAC. A completion's
// 0230 that fails its MAC proves nothing, so the completion stays an unknown outcome.
func (s *Service) outcome(ctx context.Context, p sendParams, resp map[int]string, sendErr error) (txn Transaction, macFailed bool) {
	if sendErr != nil {
		txn = Transaction{Type: p.txnType, Amount: p.requestedAmt}
		txn.Status, txn.ResponseCode = purchase.SendFailureStatus(sendErr)
		return txn, false
	}
	txn = mapResponseToTransaction(p.txnType, p.requestedAmt, resp)
	if s.mac.Verify(ctx, responseMTI(p.mti), resp) {
		return txn, false
	}
	obs.MacFailureTotal.Inc()
	if p.txnType == tranTypeCompletion {
		return Transaction{Type: p.txnType, Amount: p.requestedAmt, Status: statusTimedOut}, true
	}
	return Transaction{Type: p.txnType, Amount: p.requestedAmt, Status: statusDeclined, ResponseCode: rcMacFailure}, true
}

// finalize records txn's status and delivers what the outcome still owes the issuer (root
// CLAUDE.md §6.4), never a resend of the request:
//   - an unknown pre-auth or refund is reversed with a 0420 (DE 39 "68"), a bad-MAC one with "06";
//   - an unknown completion is an advice: its 0220 is repeated as 0221 until the 0230, never
//     reversed (docs/03 §7.4);
//   - a balance inquiry holds no money, so it owes nothing.
//
// The follow-up is attempted even when the status update failed, from the state the row still
// holds, so a database hiccup never leaves it undone.
func (s *Service) finalize(ctx context.Context, p sendParams, row store.TranLogRow, txn Transaction, macFailed bool) (Transaction, error) {
	current, recordErr := s.recordStatus(ctx, row.ID, txn)
	if errors.Is(recordErr, store.ErrStateMoved) {
		// The orphan sweeper gave up on this request and followed it up; the caller is answered
		// from the row it left.
		return Transaction{}, recordErr
	}
	recordErr = errors.Join(recordErr, s.recordAmounts(ctx, row.ID, txn))
	switch {
	case p.txnType == tranTypeBalance:
		return txn, recordErr
	case txn.Status == statusDeclined && !macFailed:
		return txn, errors.Join(recordErr, s.releaseClaim(ctx, p))
	case p.txnType == tranTypeCompletion && txn.Status == statusTimedOut:
		if err := s.saf.QueueAdvice(ctx, row.ID, p.mti, p.fields); err != nil {
			return Transaction{}, errors.Join(recordErr, fmt.Errorf("queue %s repeat for %s: %w", p.mti, row.RRN, err))
		}
		return txn, recordErr
	}
	row.Status = current
	return s.reverseIfOwed(ctx, row, txn, macFailed, recordErr)
}

// reverseIfOwed queues the 0420 an unknown outcome or a bad MAC owes, from the state row holds.
func (s *Service) reverseIfOwed(ctx context.Context, row store.TranLogRow, txn Transaction, macFailed bool, recordErr error) (Transaction, error) {
	reason := reversalReason(txn.Status, macFailed)
	if reason == "" {
		return txn, recordErr
	}
	if err := s.saf.Queue(ctx, row, reason); err != nil {
		return Transaction{}, errors.Join(recordErr, fmt.Errorf("queue reversal %s for %s: %w", reason, row.RRN, err))
	}
	if txn.Status == statusTimedOut {
		// The response reports the row's real status (POS-G9). A MAC failure stays DECLINED RC 96
		// for the POS, as a purchase does, while its row moves on to REVERSAL_PENDING.
		txn.Status = statusReversalPending
	}
	return txn, recordErr
}

// recordStatus moves row id from SENT to txn's status and returns the status the row now holds.
func (s *Service) recordStatus(ctx context.Context, id int64, txn Transaction) (current string, err error) {
	if err := s.tranLog.UpdateStatusFrom(ctx, id, statusSent, txn.Status, txn.ResponseCode, txn.AuthCode); err != nil {
		return statusSent, fmt.Errorf("update tran_log to %s: %w", txn.Status, err)
	}
	if err := s.tranLog.RecordStateTransition(ctx, id, statusSent, txn.Status); err != nil {
		return txn.Status, fmt.Errorf("record %s->%s: %w", statusSent, txn.Status, err)
	}
	return txn.Status, nil
}

// reversalReason is DE 39 of the 0420 an outcome needs, empty when it needs none.
func reversalReason(status string, macFailed bool) string {
	switch {
	case status == statusTimedOut:
		return reasonTimeout
	case macFailed:
		return reasonMacFailure
	}
	return ""
}

// recordLinkDown logs a request the link could not carry as DECLINED RC 91: nothing was sent, so
// there is nothing to reverse (MCN-303-AC4).
func (s *Service) recordLinkDown(ctx context.Context, p sendParams, merchant store.Merchant) (Transaction, error) {
	txn := s.describe(ctx, Transaction{Type: p.txnType, Amount: p.requestedAmt, Status: statusDeclined, ResponseCode: rcLinkDown}, p, merchant)
	row := store.TranLogRow{
		RRN: p.rrn, Type: p.txnType, Status: statusDeclined, Amount: p.requestedAmt.Amount, Currency: p.requestedAmt.Currency,
		MaskedPAN: p.maskedPAN, TerminalID: p.terminalID, MerchantID: merchant.MID, ResponseCode: rcLinkDown, MTI: p.mti,
		OriginalRRN: p.originalRRN, TraceID: txn.TraceID, BusinessDate: p.businessDate,
	}
	if _, err := s.tranLog.Insert(ctx, row); err != nil {
		return Transaction{}, fmt.Errorf("insert tran_log for link-down decline: %w", err)
	}
	return txn, nil
}

func (s *Service) transition(ctx context.Context, id int64, from, to, responseCode, authCode string) error {
	if err := s.tranLog.UpdateStatus(ctx, id, to, responseCode, authCode); err != nil {
		return fmt.Errorf("update tran_log to %s: %w", to, err)
	}
	if err := s.tranLog.RecordStateTransition(ctx, id, from, to); err != nil {
		return fmt.Errorf("record %s->%s: %w", from, to, err)
	}
	return nil
}

// describe fills the fields every response carries, whatever the outcome.
func (s *Service) describe(ctx context.Context, txn Transaction, p sendParams, merchant store.Merchant) Transaction {
	txn.RRN, txn.STAN, txn.MaskedPAN, txn.TerminalID, txn.OriginalRRN = p.rrn, p.stan, p.maskedPAN, p.terminalID, p.originalRRN
	txn.MerchantName = merchant.Name
	txn.ResponseLabel = purchase.ResponseLabel(txn.ResponseCode)
	txn.TraceID = obs.TraceID(ctx)
	now := time.Now().UTC()
	txn.CreatedAt = now
	txn.BusinessDate = bizdate.Format(p.businessDate)
	return txn
}

// responseMTI is the response to request mti: the function digit plus one (0100 -> 0110).
func responseMTI(mti string) string {
	return mti[:2] + string(mti[2]+1) + mti[3:]
}

// mapResponseToTransaction reads DE 39 (RC) and DE 4 (amount) from resp - the single shared
// place partial-approval mapping happens (docs/plans/MCN-603.md Task 2: RC 10 sets
// ApprovedAmount, distinct from requestedAmount, not duplicated per flow).
func mapResponseToTransaction(txnType string, requestedAmount Money, resp map[int]string) Transaction {
	txn := Transaction{Type: txnType, Amount: requestedAmount, ResponseCode: purchase.ResponseCodeOf(resp), AuthCode: resp[38]}
	switch txn.ResponseCode {
	case "00":
		txn.Status = statusApproved
	case rcPartialApproval:
		txn.Status = statusApproved
	default:
		txn.Status = statusDeclined
	}
	if txn.ResponseCode == rcPartialApproval {
		if amt, ok := parseDE4(resp[4]); ok {
			txn.ApprovedAmount = &Money{Amount: amt, Currency: requestedAmount.Currency}
		}
	}
	if de54 := resp[54]; de54 != "" {
		if balance, ok := parseDE54(de54); ok {
			txn.Balance = &balance
		}
	}
	return txn
}

// parseDE4 parses DE 4's fixed 12-digit minor-units amount (root CLAUDE.md §6 rule 1: int64,
// never a float).
func parseDE4(de4 string) (int64, bool) {
	if de4 == "" {
		return 0, false
	}
	amt, err := strconv.ParseInt(de4, 10, 64)
	if err != nil {
		return 0, false
	}
	return amt, true
}

// parseDE54 parses the balance-inquiry sub-format this gateway defines for DE 54 (docs/03 §7.3
// leaves the sub-layout to the implementation, beyond "an..120 LLLVAR, balance inquiry
// response"): 3-digit currency code + 1-char D/C indicator + 12-digit minor-units amount.
func parseDE54(de54 string) (Money, bool) {
	if len(de54) != 16 {
		return Money{}, false
	}
	currency := de54[0:3]
	amt, err := strconv.ParseInt(de54[4:16], 10, 64)
	if err != nil {
		return Money{}, false
	}
	return Money{Amount: amt, Currency: currency}, true
}

// attachMAC computes the Retail MAC over fields for the given mti (MCN-502), same as
// purchase.Service.attachMAC.
func attachMAC(hsmModule hsm.Module, zak []byte, mti string, fields map[int]string) error {
	packed, err := iso8583.Pack(mti, fields)
	if err != nil {
		return fmt.Errorf("pack for MAC: %w", err)
	}
	mac, err := hsmModule.ComputeMAC([]byte(packed), zak)
	if err != nil {
		return fmt.Errorf("compute MAC: %w", err)
	}
	fields[64] = strings.ToUpper(hex.EncodeToString(mac))
	return nil
}

func hashRequest(v any) string {
	body, _ := json.Marshal(v)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
