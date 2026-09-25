package purchase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "time/tzdata" // Asia/Ho_Chi_Minh must resolve even on a minimal container image

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/iso8583"
	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/journey"
	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	acquirerID     = "970499" // docs/03 §3: Lab acquirer institution ID (DE 32)
	purchaseRoute  = "purchases"
	cancelRoute    = "cancellations"
	requestTimeout = 30 * time.Second // docs/03 §9

	tranTypePurchase = "PURCHASE"

	statusCreated         = "CREATED"
	statusSent            = "SENT"
	statusApproved        = "APPROVED"
	statusDeclined        = "DECLINED"
	statusTimedOut        = "TIMED_OUT"
	statusReversalPending = "REVERSAL_PENDING"

	rcLinkDown = "91" // docs/03 §8: Issuer or switch inoperative

	reasonTimeout      = "68" // docs/03 §7.3 DE 39: response arrived too late / timeout
	reasonCancellation = "17" // docs/03 §7.3 DE 39: cancelled by customer
	reasonMacFailure   = "06" // docs/03 §7.3 DE 39: error (reused for a bad incoming MAC)

	rcMacFailure  = "96" // docs/03 §11: MAC verification failed
	rcFormatError = "30" // docs/03 §8: a response without DE 39 is malformed
	responseMTI   = "0210"

	eventCreated = "transaction.created"
	eventUpdated = "transaction.updated"

	statusCreatedHTTP  = 201
	statusAcceptedHTTP = 202
)

// MerchantResolver maps a terminal to the merchant it belongs to (contracts/fixtures/cards.json
// `terminals`); *store.TerminalRepository implements it. An unknown terminal is
// store.ErrUnknownTerminal, so nothing is sent for a terminal the acquirer does not own.
type MerchantResolver interface {
	Merchant(ctx context.Context, tid string) (store.Merchant, error)
}

var terminalLocation = mustLoadLocation("Asia/Ho_Chi_Minh")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// entryModeToDE22 maps contracts/openapi.yaml's EntryMode enum to DE 22 (docs/03 §3).
var entryModeToDE22 = map[string]string{
	"CHIP_PIN":      "051",
	"CHIP_NO_PIN":   "052",
	"MANUAL_PIN":    "011",
	"MANUAL_NO_PIN": "012",
}

// ErrUnknownCardToken is returned when a PurchaseRequest names a cardToken absent from
// contracts/fixtures/cards.json.
var ErrUnknownCardToken = errors.New("unknown card token")

// Money mirrors contracts/openapi.yaml's Money schema: amount is always integer minor units.
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// PurchaseRequest mirrors contracts/openapi.yaml's PurchaseRequest schema.
//
//nolint:revive // named PurchaseRequest, not Request, to match the OpenAPI schema name exactly.
type PurchaseRequest struct {
	TerminalID        string `json:"terminalId"`
	CardToken         string `json:"cardToken"`
	EntryMode         string `json:"entryMode"`
	EncryptedPinBlock string `json:"encryptedPinBlock,omitempty"`
	EMVData           string `json:"emvData,omitempty"`
	Amount            Money  `json:"amount"`
}

// Transaction mirrors contracts/openapi.yaml's Transaction schema.
type Transaction struct {
	RRN            string    `json:"rrn"`
	STAN           string    `json:"stan,omitempty"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	ResponseCode   string    `json:"responseCode,omitempty"`
	ResponseLabel  *string   `json:"responseLabel"`
	Amount         Money     `json:"amount"`
	MaskedPAN      string    `json:"maskedPan"`
	TerminalID     string    `json:"terminalId"`
	MerchantName   string    `json:"merchantName"`
	LatencyMs      *int      `json:"latencyMs"`
	CreatedAt      time.Time `json:"createdAt"`
	ReversalReason string    `json:"reversalReason,omitempty"`
	AuthCode       string    `json:"authCode,omitempty"`
	BusinessDate   string    `json:"businessDate"`
	TraceID        string    `json:"traceId,omitempty"`
}

// ResponseLabel is the plain-language label of rc (docs/04 §2 display modes), nil when there is
// no RC yet.
func ResponseLabel(rc string) *string {
	if rc == "" {
		return nil
	}
	label := journey.EasyTextForRC(rc)
	return &label
}

// ResponseCodeOf reads DE 39 of an issuer response. A response without one is malformed, so it is
// declined as a format error rather than recorded with no RC (OVW-G11).
func ResponseCodeOf(resp map[int]string) string {
	if rc := resp[39]; rc != "" {
		return rc
	}
	return rcFormatError
}

// transactionFromRow is the Transaction a stored row reports, for events after creation.
func transactionFromRow(row store.TranLogRow) Transaction {
	return Transaction{
		RRN: row.RRN, STAN: row.NetworkSTAN, Type: row.Type, Status: row.Status, ResponseCode: row.ResponseCode,
		ResponseLabel: ResponseLabel(row.ResponseCode), Amount: Money{Amount: row.Amount, Currency: row.Currency},
		MaskedPAN: row.MaskedPAN, TerminalID: row.TerminalID, MerchantName: row.MerchantName,
		LatencyMs: journey.LatencyMs(row), CreatedAt: row.CreatedAt, ReversalReason: store.ReversalReasonOf(row.ReversalReasonCode),
		AuthCode: row.AuthCode, BusinessDate: row.CreatedAt.UTC().Format("2006-01-02"), TraceID: row.TraceID,
	}
}

// MuxSender sends a request on the acquirer's live issuer connection. *isonet.Supervisor
// satisfies it (it shares MCN-202/204's one connection rather than opening a second one).
type MuxSender interface {
	NextSTAN() (stan string, ok bool)
	Send(ctx context.Context, mti string, fields map[int]string) (map[int]string, error)
}

// LinkStatusPort reports whether the issuer connection is currently live. *isonet.Supervisor
// satisfies it.
type LinkStatusPort interface {
	IsSignedOn() bool
}

// TranLogPort persists tran_log and tran_state_history. *store.TranLogRepository satisfies it.
type TranLogPort interface {
	Insert(ctx context.Context, row store.TranLogRow) (int64, error)
	UpdateStatus(ctx context.Context, id int64, status, responseCode, authCode string) error
	RecordStateTransition(ctx context.Context, id int64, fromStatus, toStatus string) error
	UpdateLateResponse(ctx context.Context, rrn, responseCode string) error
}

// NetworkEventRecorder persists a network_event row. *store.LinkRepository satisfies it.
type NetworkEventRecorder interface {
	RecordEvent(ctx context.Context, severity, easyText, technicalText string) error
}

// HubPort broadcasts a transaction event. internal/ws.Hub (extended with BroadcastTransaction)
// satisfies it.
type HubPort interface {
	BroadcastTransaction(eventType string, txn Transaction)
	BroadcastNetworkEvent(evt store.NetworkEvent)
}

// ReversalQueuer atomically moves a transaction to REVERSAL_PENDING and enqueues its 0420
// advice. *saf.ReversalQueuer satisfies it. reasonCode is DE 39 on the 0420 (docs/03 §7.3):
// "68" timeout, "17" POS cancellation.
type ReversalQueuer interface {
	Queue(ctx context.Context, txn store.TranLogRow, reasonCode string) error
}

// TranLogGetter looks up a transaction by RRN. *store.TranLogRepository satisfies it (it already
// implements TranLogPort too).
type TranLogGetter interface {
	Get(ctx context.Context, rrn string) (store.TranLogRow, error)
}

// Service builds and sends purchase transactions.
type Service struct {
	mux           MuxSender
	linkStatus    LinkStatusPort
	cardTokens    *CardTokenRegistry
	merchants     MerchantResolver
	tranLog       TranLogPort
	tranLogGet    TranLogGetter
	idempotency   IdempotencyPort
	hub           HubPort
	reversal      ReversalQueuer
	hsm           hsm.Module
	zak           []byte
	mac           MACVerifier
	events        NetworkEventRecorder
	duplicateHook func() bool
}

// SetChaosDuplicateHook installs fn, checked once per purchase send: when it returns true, the
// same 0200 (same STAN) is fired twice before the response is processed, exercising the issuer's
// Deduplicate participant end-to-end (MCN-404's DUPLICATE_REQUEST scenario). A nil hook (the
// default) never duplicates.
func (s *Service) SetChaosDuplicateHook(fn func() bool) { s.duplicateHook = fn }

// NewService builds a Service. mux also serves as the LinkStatusPort (e.g. *isonet.Supervisor
// implements both). tranLog also serves as the TranLogGetter (*store.TranLogRepository
// implements both). zak is the clear ZAK used for the Retail MAC (MCN-502-AC1/AC2), unwrapped
// once at construction - not per-request, matching how Service already holds its other
// long-lived dependencies. keyStore backs the dual-key acceptance retry (MCN-504-AC2); a nil
// keyStore simply disables the retry (existing single-key MAC verification, unchanged). events
// persists the late-response network event.
func NewService(mux interface {
	MuxSender
	LinkStatusPort
}, cardTokens *CardTokenRegistry, merchants MerchantResolver, tranLog interface {
	TranLogPort
	TranLogGetter
}, idempotency IdempotencyPort, hub HubPort, reversal ReversalQueuer, hsmModule hsm.Module, zak []byte, keyStore RetiredKeyFinder, events NetworkEventRecorder) *Service {
	return &Service{mux: mux, linkStatus: mux, cardTokens: cardTokens, merchants: merchants, tranLog: tranLog, tranLogGet: tranLog, idempotency: idempotency, hub: hub, reversal: reversal, hsm: hsmModule, zak: zak, mac: NewMACVerifier(hsmModule, zak, keyStore), events: events}
}

// CreatePurchase builds a 0200, sends it through the live issuer connection, persists every
// state transition, and returns the resulting Transaction. A replayed idempotencyKey with the
// same route returns the previously stored Transaction unchanged, without resending anything.
func (s *Service) CreatePurchase(ctx context.Context, req PurchaseRequest, idempotencyKey string) (Transaction, error) {
	return Idempotent(ctx, s.idempotency, idempotencyKey, purchaseRoute, hashRequest(req), statusCreatedHTTP, func() (Transaction, error) {
		return s.createPurchase(ctx, req)
	})
}

func (s *Service) createPurchase(ctx context.Context, req PurchaseRequest) (Transaction, error) {
	card, merchant, err := s.resolveCardAndMerchant(ctx, req)
	if err != nil {
		return Transaction{}, err
	}
	maskedPAN := obs.MaskPAN(card.PAN)

	stan, ok := s.mux.NextSTAN()
	if !s.linkStatus.IsSignedOn() || !ok {
		txn := s.declinedTransaction(ctx, req, merchant, maskedPAN, rcLinkDown)
		if err := s.recordLinkDown(ctx, txn, req, merchant); err != nil {
			return Transaction{}, err
		}
		s.hub.BroadcastTransaction(eventCreated, txn)
		return txn, nil
	}

	// Idempotent stores the response on a detached context, so a caller that gives up once the 0200
	// may be at the issuer can't make a retry with the same key send a second 0200.
	txn, err := s.sendPurchase(ctx, req, card, merchant, maskedPAN, stan)
	if err != nil {
		return Transaction{}, err
	}
	s.hub.BroadcastTransaction(eventCreated, txn)
	return txn, nil
}

// persistTimeout bounds the work a request finishes after its send on a detached context (root
// CLAUDE.md §6.9: every path has a cancellation path).
const persistTimeout = 10 * time.Second

// Detach returns a context that outlives ctx's cancellation, for recording the outcome of a
// request that may already be at the issuer, bounded by persistTimeout.
func Detach(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
}

// resolveCardAndMerchant maps the request's cardToken to its fixture card and its terminal to
// the owning merchant; either being unknown means nothing is sent.
func (s *Service) resolveCardAndMerchant(ctx context.Context, req PurchaseRequest) (CardFixture, store.Merchant, error) {
	card, ok := s.cardTokens.Resolve(req.CardToken)
	if !ok {
		return CardFixture{}, store.Merchant{}, ErrUnknownCardToken
	}
	merchant, err := s.merchants.Merchant(ctx, req.TerminalID)
	if err != nil {
		return CardFixture{}, store.Merchant{}, fmt.Errorf("resolve merchant for terminal %s: %w", req.TerminalID, err)
	}
	return card, merchant, nil
}

// sendPurchase builds the 0200, sends it, and maps the outcome to a Transaction, recording every
// tran_log state transition along the way.
func (s *Service) sendPurchase(ctx context.Context, req PurchaseRequest, card CardFixture, merchant store.Merchant, maskedPAN, stan string) (Transaction, error) {
	now := time.Now().UTC()
	local := now.In(terminalLocation)
	rrn := BuildRRN(now, stan)

	fields := map[int]string{
		2:  card.PAN,
		3:  "000000", // docs/03 §6: purchase
		4:  fmt.Sprintf("%012d", req.Amount.Amount),
		7:  now.Format("0102150405"),
		11: stan,
		12: local.Format("150405"),
		13: local.Format("0102"),
		14: card.ExpiryYYMM,
		15: now.Format("0102"),
		22: entryModeToDE22[req.EntryMode],
		32: acquirerID,
		37: rrn,
		41: req.TerminalID,
		42: merchant.MID,
		49: req.Amount.Currency,
	}

	row := store.TranLogRow{
		RRN: rrn, Type: tranTypePurchase, Status: statusCreated, Amount: req.Amount.Amount, Currency: req.Amount.Currency,
		MaskedPAN: maskedPAN, TerminalID: req.TerminalID, MerchantID: merchant.MID, NetworkSTAN: stan, MTI: "0200",
		ProcessingCode: fields[3], POSEntryMode: fields[22], SentAt: &now, CardToken: req.CardToken, TraceID: obs.TraceID(ctx),
	}
	id, err := s.tranLog.Insert(ctx, row)
	if err != nil {
		return Transaction{}, fmt.Errorf("insert tran_log: %w", err)
	}
	if err := s.tranLog.UpdateStatus(ctx, id, statusSent, "", ""); err != nil {
		return Transaction{}, fmt.Errorf("update tran_log to %s: %w", statusSent, err)
	}
	if err := s.tranLog.RecordStateTransition(ctx, id, statusCreated, statusSent); err != nil {
		return Transaction{}, fmt.Errorf("record %s->%s: %w", statusCreated, statusSent, err)
	}

	if err := s.attachMAC(fields); err != nil {
		return Transaction{}, fmt.Errorf("compute outgoing MAC: %w", err)
	}

	s.sendDuplicateIfActive(ctx, fields)

	sendCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	sendStart := time.Now()
	resp, err := s.mux.Send(sendCtx, "0200", fields)
	latencyMs := int(time.Since(sendStart).Milliseconds())

	// The request may already be at the issuer: whatever happened to the caller's request, the
	// outcome must be recorded, so nothing after the send may be cancelled with it.
	ctx, cancelPersist := Detach(ctx)
	defer cancelPersist()
	txn := s.baseTransaction(ctx, req, merchant, rrn, stan, maskedPAN)
	macFailed := s.mapSendOutcome(ctx, &txn, resp, err)
	if err == nil {
		txn.LatencyMs = &latencyMs
	}
	txn.ResponseLabel = ResponseLabel(txn.ResponseCode)

	row.ID, row.CreatedAt = id, now
	finalized, err := s.finalizeSendResult(ctx, row, txn, macFailed)
	return finalized, afterSend(err)
}

// mapSendOutcome maps mux.Send's (resp, sendErr) onto txn's Status/ResponseCode/AuthCode -
// a failed send (see SendFailureStatus), or a normal response (with its incoming MAC verified,
// and on mismatch overridden to RC 96 / DECLINED, mcn_mac_failure_total incremented) - and
// reports whether the incoming MAC failed.
func (s *Service) mapSendOutcome(ctx context.Context, txn *Transaction, resp map[int]string, sendErr error) (macFailed bool) {
	if sendErr != nil {
		txn.Status, txn.ResponseCode = SendFailureStatus(sendErr)
		return false
	}

	txn.ResponseCode = ResponseCodeOf(resp)
	txn.AuthCode = resp[38]
	if txn.ResponseCode == "00" {
		txn.Status = statusApproved
	} else {
		txn.Status = statusDeclined
	}
	if s.mac.Verify(ctx, responseMTI, resp) {
		return false
	}
	txn.Status = statusDeclined
	txn.ResponseCode = rcMacFailure
	obs.MacFailureTotal.Inc()
	return true
}

// SendFailureStatus is what a transaction becomes when mux.Send fails. Only a link that was down
// before the write guarantees nothing left the gateway: that is a DECLINED RC 91. Every other
// failure (timeout, broken connection, cancelled request) is an unknown outcome, so the row goes
// TIMED_OUT and the caller queues a reversal (root CLAUDE.md §6.4); it never resends. Mux.Send's
// other pre-write failure, "pack request", can't happen here: every caller packs the same fields
// to compute the MAC before sending, so a message that doesn't pack never reaches Send.
func SendFailureStatus(sendErr error) (status, responseCode string) {
	if errors.Is(sendErr, isonet.ErrNotSignedOn) {
		return statusDeclined, rcLinkDown
	}
	return statusTimedOut, ""
}

// attachMAC computes the Retail MAC over fields (MTI 0200, not yet containing DE 64/128) and
// sets DE 64 with the result, uppercase hex (MCN-502's Ruling 2 - DE 128 is unneeded here since
// this service never sets a field above 64 itself).
func (s *Service) attachMAC(fields map[int]string) error {
	packed, err := iso8583.Pack("0200", fields)
	if err != nil {
		return fmt.Errorf("pack for MAC: %w", err)
	}
	mac, err := s.hsm.ComputeMAC([]byte(packed), s.zak)
	if err != nil {
		return fmt.Errorf("compute MAC: %w", err)
	}
	fields[64] = strings.ToUpper(hex.EncodeToString(mac))
	return nil
}

// sendDuplicateIfActive fires a blocking duplicate with the same STAN/DE37 before the "real"
// send in sendPurchase, when the MCN-404 chaos DUPLICATE_REQUEST scenario is active: the
// issuer's Deduplicate participant should recognize the repeat and never double-post the ledger
// entry. Its response is discarded - only the send in sendPurchase drives txn.Status. Sequential
// (not fire-and-forget) per gateway-go/CLAUDE.md's goroutine-ownership rule.
func (s *Service) sendDuplicateIfActive(ctx context.Context, fields map[int]string) {
	if s.duplicateHook == nil || !s.duplicateHook() {
		return
	}
	dupCtx, dupCancel := context.WithTimeout(ctx, requestTimeout)
	defer dupCancel()
	_, _ = s.mux.Send(dupCtx, "0200", fields)
}

// finalizeSendResult persists the post-send status and queues the reversal an unknown outcome
// (DE 39 "68") or a bad incoming MAC ("06", MCN-502-AC3) needs - unknown outcome -> reversal (root
// CLAUDE.md §6.4): never resend the 0200. The reversal is attempted even when the status update
// failed, from the state the row still holds, so a database hiccup never leaves money unreturned.
// The response reports the row's real status (docs/04 §4), REVERSAL_PENDING after a timeout.
func (s *Service) finalizeSendResult(ctx context.Context, row store.TranLogRow, txn Transaction, macFailed bool) (Transaction, error) {
	current, recordErr := s.recordStatus(ctx, row.ID, txn)
	reason := reversalReasonFor(txn.Status, macFailed)
	if reason == "" {
		return txn, recordErr
	}
	row.Status = current
	if err := s.reversal.Queue(ctx, row, reason); err != nil {
		return Transaction{}, errors.Join(recordErr, fmt.Errorf("queue reversal %s: %w", reason, err))
	}
	if txn.Status == statusTimedOut {
		txn.Status = statusReversalPending
	}
	return txn, recordErr
}

// recordStatus moves row id from SENT to txn's status and returns the status the row now holds.
func (s *Service) recordStatus(ctx context.Context, id int64, txn Transaction) (current string, err error) {
	if err := s.tranLog.UpdateStatus(ctx, id, txn.Status, txn.ResponseCode, txn.AuthCode); err != nil {
		return statusSent, fmt.Errorf("update tran_log to %s: %w", txn.Status, err)
	}
	if err := s.tranLog.RecordStateTransition(ctx, id, statusSent, txn.Status); err != nil {
		return txn.Status, fmt.Errorf("record %s->%s: %w", statusSent, txn.Status, err)
	}
	return txn.Status, nil
}

// reversalReasonFor is DE 39 of the 0420 an outcome needs, empty when it needs none.
func reversalReasonFor(status string, macFailed bool) string {
	switch {
	case status == statusTimedOut:
		return reasonTimeout
	case macFailed:
		return reasonMacFailure
	}
	return ""
}

func (s *Service) baseTransaction(ctx context.Context, req PurchaseRequest, merchant store.Merchant, rrn, stan, maskedPAN string) Transaction {
	now := time.Now().UTC()
	return Transaction{
		RRN: rrn, STAN: stan, Type: tranTypePurchase, Amount: req.Amount, MaskedPAN: maskedPAN,
		TerminalID: req.TerminalID, MerchantName: merchant.Name, CreatedAt: now,
		BusinessDate: now.Format("2006-01-02"), TraceID: obs.TraceID(ctx),
	}
}

// declinedTransaction builds a decline for a purchase that was never sent (no STAN was ever
// allocated, since mux.Send is never called on this path). Its RRN uses STAN "000000" per
// docs/03 §5's format - ponytail: collisions are possible if two link-down declines land in the
// same UTC second, acceptable for this lab; a real deployment would reserve a STAN even for
// link-down declines.
func (s *Service) declinedTransaction(ctx context.Context, req PurchaseRequest, merchant store.Merchant, maskedPAN, responseCode string) Transaction {
	const noStan = "000000"
	rrn := BuildRRN(time.Now().UTC(), noStan)
	txn := s.baseTransaction(ctx, req, merchant, rrn, noStan, maskedPAN)
	txn.Status = statusDeclined
	txn.ResponseCode = responseCode
	txn.ResponseLabel = ResponseLabel(responseCode)
	return txn
}

// recordLinkDown logs a link-down decline in tran_log: nothing was sent, so sendPurchase never
// logged it.
func (s *Service) recordLinkDown(ctx context.Context, txn Transaction, req PurchaseRequest, merchant store.Merchant) error {
	row := store.TranLogRow{
		RRN: txn.RRN, Type: tranTypePurchase, Status: statusDeclined, MTI: "0200",
		Amount: req.Amount.Amount, Currency: req.Amount.Currency, MaskedPAN: txn.MaskedPAN,
		TerminalID: req.TerminalID, MerchantID: merchant.MID, ResponseCode: txn.ResponseCode, TraceID: txn.TraceID,
	}
	if _, err := s.tranLog.Insert(ctx, row); err != nil {
		return fmt.Errorf("insert tran_log for link-down decline: %w", err)
	}
	return nil
}

// CancelPurchase queues a POS-initiated reversal (DE 39 = "17") for the transaction identified
// by rrn, through the same atomic path CreatePurchase's timeout branch uses.
func (s *Service) CancelPurchase(ctx context.Context, rrn string, idempotencyKey string) (Transaction, error) {
	return Idempotent(ctx, s.idempotency, idempotencyKey, cancelRoute, sha256Hex(rrn), statusAcceptedHTTP, func() (Transaction, error) {
		return s.cancelPurchase(ctx, rrn)
	})
}

func (s *Service) cancelPurchase(ctx context.Context, rrn string) (Transaction, error) {
	row, err := s.tranLogGet.Get(ctx, rrn)
	if err != nil {
		return Transaction{}, fmt.Errorf("look up transaction %s: %w", rrn, err)
	}
	// Only an approval holds the cardholder's money. Declines (link-down ones were never even
	// sent) have nothing to return; timeouts and MAC failures already queued their own reversal.
	// Pre-auths and completions are not 0200s, so a 0420 naming a 0200 original would miss them.
	if row.Type != tranTypePurchase || row.Status != statusApproved {
		return Transaction{}, fmt.Errorf("cancel %s (%s): %w", rrn, row.Status, store.ErrNotReversible)
	}
	if err := s.reversal.Queue(ctx, row, reasonCancellation); err != nil {
		return Transaction{}, fmt.Errorf("queue reversal for cancellation: %w", err)
	}

	row.Status = statusReversalPending
	row.ReversalReasonCode = reasonCancellation
	txn := transactionFromRow(row)
	s.hub.BroadcastTransaction(eventUpdated, txn)
	return txn, nil
}

// BroadcastUpdate announces rrn's current state as transaction.updated (OVW-G3): every status
// change after creation that isn't made by this service, such as the SAF acknowledging a reversal.
func (s *Service) BroadcastUpdate(ctx context.Context, rrn string) error {
	row, err := s.tranLogGet.Get(ctx, rrn)
	if err != nil {
		return fmt.Errorf("look up transaction %s: %w", rrn, err)
	}
	s.hub.BroadcastTransaction(eventUpdated, transactionFromRow(row))
	return nil
}

// RecordLateResponse records a 0210 that arrived for rrn after its transaction already left
// SENT (timed out, reversed, ...). It never changes tran_log.state - the transaction is already
// final - only the late-response columns, a metric (incremented by the isonet.Mux late-response
// hook, not here) and a WARN network event (MCN-403-AC1). A response that is still on time
// (status == SENT) is not this story's path and is left alone.
func (s *Service) RecordLateResponse(ctx context.Context, rrn, responseCode string) error {
	row, err := s.tranLogGet.Get(ctx, rrn)
	if err != nil {
		return fmt.Errorf("look up transaction %s: %w", rrn, err)
	}
	if row.Status == statusSent {
		return nil
	}
	if err := s.tranLog.UpdateLateResponse(ctx, rrn, responseCode); err != nil {
		return fmt.Errorf("update late response for %s: %w", rrn, err)
	}
	evt := store.NetworkEvent{
		Severity:      "WARN",
		EasyText:      "A response arrived too late for a transaction",
		TechnicalText: fmt.Sprintf("late 0210 for RRN %s, RC %s", rrn, responseCode),
	}
	if err := s.events.RecordEvent(ctx, evt.Severity, evt.EasyText, evt.TechnicalText); err != nil {
		return fmt.Errorf("record late-response event for %s: %w", rrn, err)
	}
	s.hub.BroadcastNetworkEvent(evt)
	return s.BroadcastUpdate(ctx, rrn)
}

func hashRequest(req PurchaseRequest) string {
	body, _ := json.Marshal(req)
	return sha256Hex(string(body))
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
