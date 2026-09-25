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

	rcMacFailure = "96" // docs/03 §11: MAC verification failed
	responseMTI  = "0210"

	// dualKeyWindow is MCN-504-AC2's grace period: a ZAK retired by a rotation is still accepted
	// for this long after retirement (docs/03 §9 "Reversal grace for new key (DE 70 = 161)").
	dualKeyWindow = 5 * time.Minute
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
	RRN          string    `json:"rrn"`
	STAN         string    `json:"stan,omitempty"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	ResponseCode string    `json:"responseCode,omitempty"`
	Amount       Money     `json:"amount"`
	MaskedPAN    string    `json:"maskedPan"`
	TerminalID   string    `json:"terminalId"`
	MerchantName string    `json:"merchantName"`
	CreatedAt    time.Time `json:"createdAt"`
	AuthCode     string    `json:"authCode,omitempty"`
	BusinessDate string    `json:"businessDate"`
	TraceID      string    `json:"traceId"`
}

// RetiredKeyFinder looks up a recently-retired key, backing MAC dual-key acceptance during a
// rotation's grace window (MCN-504-AC2). *store.KeyStoreRepository satisfies it.
type RetiredKeyFinder interface {
	FindRecentlyRetired(ctx context.Context, keyType, ownerRef string, within time.Duration) (*store.KeyRow, error)
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

// IdempotencyPort persists idempotency_record. *store.IdempotencyRepository satisfies it.
type IdempotencyPort interface {
	Find(ctx context.Context, key, route string) (*store.StoredResponse, error)
	Store(ctx context.Context, key, route, requestHash string, status int, body []byte) error
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
	keyStore      RetiredKeyFinder
	ownerRef      string
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
// keyStore simply disables the retry (existing single-key MAC verification, unchanged).
func NewService(mux interface {
	MuxSender
	LinkStatusPort
}, cardTokens *CardTokenRegistry, merchants MerchantResolver, tranLog interface {
	TranLogPort
	TranLogGetter
}, idempotency IdempotencyPort, hub HubPort, reversal ReversalQueuer, hsmModule hsm.Module, zak []byte, keyStore RetiredKeyFinder) *Service {
	return &Service{mux: mux, linkStatus: mux, cardTokens: cardTokens, merchants: merchants, tranLog: tranLog, tranLogGet: tranLog, idempotency: idempotency, hub: hub, reversal: reversal, hsm: hsmModule, zak: zak, keyStore: keyStore}
}

// CreatePurchase builds a 0200, sends it through the live issuer connection, persists every
// state transition, and returns the resulting Transaction. A replayed idempotencyKey with the
// same route returns the previously stored Transaction unchanged, without resending anything.
func (s *Service) CreatePurchase(ctx context.Context, req PurchaseRequest, idempotencyKey string) (Transaction, error) {
	requestHash := hashRequest(req)

	if stored, err := s.idempotency.Find(ctx, idempotencyKey, purchaseRoute); err != nil {
		return Transaction{}, fmt.Errorf("check idempotency: %w", err)
	} else if stored != nil {
		var txn Transaction
		if err := json.Unmarshal(stored.Body, &txn); err != nil {
			return Transaction{}, fmt.Errorf("decode stored transaction: %w", err)
		}
		return txn, nil
	}

	card, merchant, err := s.resolveCardAndMerchant(ctx, req)
	if err != nil {
		return Transaction{}, err
	}
	maskedPAN := obs.MaskPAN(card.PAN)

	stan, ok := s.mux.NextSTAN()
	if !s.linkStatus.IsSignedOn() || !ok {
		txn := s.declinedTransaction(req, merchant, maskedPAN, rcLinkDown)
		if err := s.persistAndBroadcast(ctx, txn, req, merchant, idempotencyKey, requestHash); err != nil {
			return Transaction{}, err
		}
		return txn, nil
	}

	txn, err := s.sendPurchase(ctx, req, card, merchant, maskedPAN, stan)
	if err != nil {
		return Transaction{}, err
	}
	if err := s.persistAndBroadcast(ctx, txn, req, merchant, idempotencyKey, requestHash); err != nil {
		return Transaction{}, err
	}
	return txn, nil
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
		MaskedPAN: maskedPAN, TerminalID: req.TerminalID, MerchantID: merchant.MID, NetworkSTAN: stan,
		ProcessingCode: fields[3], POSEntryMode: fields[22], SentAt: &now, CardToken: req.CardToken,
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
	resp, err := s.mux.Send(sendCtx, "0200", fields)

	txn := s.baseTransaction(req, merchant, rrn, stan, maskedPAN)
	macFailed, err := s.mapSendOutcome(ctx, &txn, resp, err)
	if err != nil {
		return Transaction{}, err
	}

	finalized, err := s.finalizeSendResult(ctx, id, row, txn, now)
	if err != nil {
		return Transaction{}, err
	}
	if macFailed {
		if err := s.queueMacFailureReversal(ctx, row, id, now); err != nil {
			return Transaction{}, err
		}
	}
	return finalized, nil
}

// mapSendOutcome maps mux.Send's (resp, sendErr) onto txn's Status/ResponseCode/AuthCode -
// a timeout, a hard send error, or a normal response (with its incoming MAC verified, and on
// mismatch overridden to RC 96 / DECLINED, mcn_mac_failure_total incremented) - and reports
// whether the incoming MAC failed.
func (s *Service) mapSendOutcome(ctx context.Context, txn *Transaction, resp map[int]string, sendErr error) (macFailed bool, err error) {
	switch {
	case errors.Is(sendErr, context.DeadlineExceeded):
		txn.Status = statusTimedOut
		return false, nil
	case sendErr != nil:
		return false, fmt.Errorf("send purchase: %w", sendErr)
	}

	txn.ResponseCode = resp[39]
	txn.AuthCode = resp[38]
	if txn.ResponseCode == "00" {
		txn.Status = statusApproved
	} else {
		txn.Status = statusDeclined
	}
	if s.verifyIncomingMAC(ctx, resp) {
		return false, nil
	}
	txn.Status = statusDeclined
	txn.ResponseCode = rcMacFailure
	obs.MacFailureTotal.Inc()
	return true, nil
}

// queueMacFailureReversal queues the DE 39 "06" reversal for a purchase whose incoming MAC
// failed verification (MCN-502-AC3).
func (s *Service) queueMacFailureReversal(ctx context.Context, row store.TranLogRow, id int64, now time.Time) error {
	reversalRow := row
	reversalRow.ID = id
	reversalRow.Status = statusDeclined
	reversalRow.CreatedAt = now
	if err := s.reversal.Queue(ctx, reversalRow, reasonMacFailure); err != nil {
		return fmt.Errorf("queue reversal for mac failure: %w", err)
	}
	return nil
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

// verifyIncomingMAC recomputes the Retail MAC over resp (excluding its own DE 64/128) and
// compares it to the MAC resp carried. On a mismatch against the current ZAK, it retries once
// against the most recently retired ZAK (if any, within dualKeyWindow) - MCN-504-AC2's dual-key
// acceptance so an in-flight transaction MAC'd under the old ZAK still verifies during a
// rotation's grace window (Ruling 2). A response with no MAC field at all fails verification.
func (s *Service) verifyIncomingMAC(ctx context.Context, resp map[int]string) bool {
	macHex, macField, ok := macFieldOf(resp)
	if !ok {
		return false
	}
	respWithoutMAC := make(map[int]string, len(resp))
	for n, v := range resp {
		if n == macField {
			continue
		}
		respWithoutMAC[n] = v
	}
	packed, err := iso8583.Pack(responseMTI, respWithoutMAC)
	if err != nil {
		return false
	}
	if s.macMatches(packed, macHex, s.zak) {
		return true
	}
	return s.macMatchesRecentlyRetiredZAK(ctx, packed, macHex)
}

// macMatchesRecentlyRetiredZAK retries verification against the most recently retired ZAK, if
// one retired within dualKeyWindow (MCN-504-AC2). s.keyStore is nil in tests that don't wire
// dual-key acceptance; a nil keyStore or no recently-retired row simply skips the retry.
func (s *Service) macMatchesRecentlyRetiredZAK(ctx context.Context, packed, macHex string) bool {
	if s.keyStore == nil {
		return false
	}
	retired, err := s.keyStore.FindRecentlyRetired(ctx, "ZAK", s.ownerRef, dualKeyWindow)
	if err != nil || retired == nil {
		return false
	}
	keyUnderLMK, err := hex.DecodeString(retired.KeyUnderLMKHex)
	if err != nil {
		return false
	}
	oldZAK, err := s.hsm.Unwrap(keyUnderLMK)
	if err != nil {
		return false
	}
	return s.macMatches(packed, macHex, oldZAK)
}

func (s *Service) macMatches(packed, macHex string, zak []byte) bool {
	expected, err := s.hsm.ComputeMAC([]byte(packed), zak)
	if err != nil {
		return false
	}
	return strings.EqualFold(macHex, hex.EncodeToString(expected))
}

// macFieldOf returns whichever of DE 64/DE 128 is present in fields (MCN-502's Ruling 2: DE 128
// only when a secondary bitmap is present).
func macFieldOf(fields map[int]string) (macHex string, fieldNum int, ok bool) {
	if v, present := fields[64]; present {
		return v, 64, true
	}
	if v, present := fields[128]; present {
		return v, 128, true
	}
	return "", 0, false
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

// finalizeSendResult persists the post-send status and, on a timeout, queues the reversal -
// unknown outcome -> reversal (root CLAUDE.md §6.4): never resend the 0200, queue a 0420 in SAF
// instead. The reversal->REVERSAL_PENDING transition happens inside Queue, so txn.Status here
// stays TIMED_OUT for this response (the caller sees the queue-time state, not the tran_log row
// after Queue).
func (s *Service) finalizeSendResult(ctx context.Context, id int64, row store.TranLogRow, txn Transaction, now time.Time) (Transaction, error) {
	if err := s.tranLog.UpdateStatus(ctx, id, txn.Status, txn.ResponseCode, txn.AuthCode); err != nil {
		return Transaction{}, fmt.Errorf("update tran_log to %s: %w", txn.Status, err)
	}
	_ = s.tranLog.RecordStateTransition(ctx, id, statusSent, txn.Status)

	if txn.Status != statusTimedOut {
		return txn, nil
	}
	reversalRow := row
	reversalRow.ID = id
	reversalRow.Status = statusTimedOut
	reversalRow.CreatedAt = now
	if err := s.reversal.Queue(ctx, reversalRow, reasonTimeout); err != nil {
		return Transaction{}, fmt.Errorf("queue reversal for timeout: %w", err)
	}
	return txn, nil
}

func (s *Service) baseTransaction(req PurchaseRequest, merchant store.Merchant, rrn, stan, maskedPAN string) Transaction {
	now := time.Now().UTC()
	return Transaction{
		RRN: rrn, STAN: stan, Type: tranTypePurchase, Amount: req.Amount, MaskedPAN: maskedPAN,
		TerminalID: req.TerminalID, MerchantName: merchant.Name, CreatedAt: now,
		BusinessDate: now.Format("2006-01-02"),
	}
}

// declinedTransaction builds a decline for a purchase that was never sent (no STAN was ever
// allocated, since mux.Send is never called on this path). Its RRN uses STAN "000000" per
// docs/03 §5's format - ponytail: collisions are possible if two link-down declines land in the
// same UTC second, acceptable for this lab; a real deployment would reserve a STAN even for
// link-down declines.
func (s *Service) declinedTransaction(req PurchaseRequest, merchant store.Merchant, maskedPAN, responseCode string) Transaction {
	const noStan = "000000"
	rrn := BuildRRN(time.Now().UTC(), noStan)
	txn := s.baseTransaction(req, merchant, rrn, noStan, maskedPAN)
	txn.Status = statusDeclined
	txn.ResponseCode = responseCode
	return txn
}

// persistAndBroadcast records a link-down decline in tran_log, broadcasts the transaction, and
// stores the idempotent response so a replay never resends.
func (s *Service) persistAndBroadcast(ctx context.Context, txn Transaction, req PurchaseRequest, merchant store.Merchant, idempotencyKey, requestHash string) error {
	if txn.Status == statusDeclined && txn.ResponseCode == rcLinkDown {
		// Link-down decline: mux.Send was never called, so this row wasn't logged earlier in
		// CreatePurchase; log it now so it's still visible in tran_log.
		row := store.TranLogRow{
			RRN: txn.RRN, Type: tranTypePurchase, Status: statusDeclined,
			Amount: req.Amount.Amount, Currency: req.Amount.Currency, MaskedPAN: txn.MaskedPAN,
			TerminalID: req.TerminalID, MerchantID: merchant.MID, ResponseCode: txn.ResponseCode,
		}
		if _, err := s.tranLog.Insert(ctx, row); err != nil {
			return fmt.Errorf("insert tran_log for link-down decline: %w", err)
		}
	}

	s.hub.BroadcastTransaction("transaction.created", txn)

	body, err := json.Marshal(txn)
	if err != nil {
		return fmt.Errorf("encode transaction for idempotency store: %w", err)
	}
	if err := s.idempotency.Store(ctx, idempotencyKey, purchaseRoute, requestHash, 201, body); err != nil {
		return fmt.Errorf("store idempotent response: %w", err)
	}
	return nil
}

// CancelPurchase queues a POS-initiated reversal (DE 39 = "17") for the transaction identified
// by rrn, through the same atomic path CreatePurchase's timeout branch uses.
func (s *Service) CancelPurchase(ctx context.Context, rrn string, idempotencyKey string) (Transaction, error) {
	if stored, err := s.idempotency.Find(ctx, idempotencyKey, cancelRoute); err != nil {
		return Transaction{}, fmt.Errorf("check idempotency: %w", err)
	} else if stored != nil {
		var txn Transaction
		if err := json.Unmarshal(stored.Body, &txn); err != nil {
			return Transaction{}, fmt.Errorf("decode stored transaction: %w", err)
		}
		return txn, nil
	}

	row, err := s.tranLogGet.Get(ctx, rrn)
	if err != nil {
		return Transaction{}, fmt.Errorf("look up transaction %s: %w", rrn, err)
	}
	if err := s.reversal.Queue(ctx, row, reasonCancellation); err != nil {
		return Transaction{}, fmt.Errorf("queue reversal for cancellation: %w", err)
	}

	txn := Transaction{
		RRN: row.RRN, Type: row.Type, Status: statusReversalPending,
		Amount: Money{Amount: row.Amount, Currency: row.Currency}, MaskedPAN: row.MaskedPAN,
		TerminalID: row.TerminalID, MerchantName: row.MerchantName, CreatedAt: time.Now().UTC(),
		BusinessDate: time.Now().UTC().Format("2006-01-02"),
	}

	body, err := json.Marshal(txn)
	if err != nil {
		return Transaction{}, fmt.Errorf("encode transaction for idempotency store: %w", err)
	}
	requestHash := sha256Hex(rrn)
	if err := s.idempotency.Store(ctx, idempotencyKey, cancelRoute, requestHash, 202, body); err != nil {
		return Transaction{}, fmt.Errorf("store idempotent response: %w", err)
	}
	return txn, nil
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
	s.hub.BroadcastNetworkEvent(store.NetworkEvent{
		Severity:      "WARN",
		EasyText:      "A response arrived too late for a transaction",
		TechnicalText: fmt.Sprintf("late 0210 for RRN %s, RC %s", rrn, responseCode),
	})
	return nil
}

func hashRequest(req PurchaseRequest) string {
	body, _ := json.Marshal(req)
	return sha256Hex(string(body))
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
