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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/iso8583"
	"github.com/mcn/gateway-go/internal/purchase"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	acquirerID     = "970499" // docs/03 §3, same lab acquirer ID purchase.Service uses
	requestTimeout = 30 * time.Second

	rcPartialApproval = "10" // docs/03 C1: partial approval, DE 4 carries the approved amount

	// ponytail: v1 seeds exactly one terminal/merchant, same fixture purchase.Service uses
	// (contracts/fixtures/cards.json). Duplicated here rather than exported from purchase
	// because it's a fixture literal, not shared behavior.
	fixedMerchantID = "GOCPHO000000001"
)

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
	AuthCode       string    `json:"authCode,omitempty"`
	OriginalRRN    string    `json:"originalRrn,omitempty"`
	BusinessDate   string    `json:"businessDate"`
	CreatedAt      time.Time `json:"createdAt"`
}

// MuxSender sends a request on the acquirer's live issuer connection; *isonet.Supervisor
// satisfies it (same port purchase.Service uses).
type MuxSender interface {
	NextSTAN() (stan string, ok bool)
	Send(ctx context.Context, mti string, fields map[int]string) (map[int]string, error)
}

// TranLogPort persists tran_log rows; *store.TranLogRepository satisfies it.
type TranLogPort interface {
	Insert(ctx context.Context, row store.TranLogRow) (int64, error)
	UpdateStatus(ctx context.Context, id int64, status, responseCode, authCode string) error
}

// IdempotencyPort persists idempotency_record; *store.IdempotencyRepository satisfies it.
type IdempotencyPort interface {
	Find(ctx context.Context, key, route string) (*store.StoredResponse, error)
	Store(ctx context.Context, key, route, requestHash string, status int, body []byte) error
}

// HubPort broadcasts a transaction event; internal/ws.Hub (extended for purchase.Service)
// satisfies it.
type HubPort interface {
	BroadcastTransaction(eventType string, txn Transaction)
}

// Service builds and sends advanced (pre-auth/completion/refund/balance) transactions.
type Service struct {
	mux         MuxSender
	cardTokens  *purchase.CardTokenRegistry
	tranLog     TranLogPort
	idempotency IdempotencyPort
	hub         HubPort
	hsm         hsm.Module
	zak         []byte
}

// NewService builds a Service, reusing the same dependency shapes purchase.NewService takes
// (mux, cardTokens, tranLog, idempotency, hub, hsmModule, zak) - Ruling 2: a sibling service,
// not a re-derivation of purchase.Service's already-proven wiring.
func NewService(mux MuxSender, cardTokens *purchase.CardTokenRegistry, tranLog TranLogPort, idempotency IdempotencyPort, hub HubPort, hsmModule hsm.Module, zak []byte) *Service {
	return &Service{mux: mux, cardTokens: cardTokens, tranLog: tranLog, idempotency: idempotency, hub: hub, hsm: hsmModule, zak: zak}
}

// sendParams carries what each flow-specific Create* method fills in before calling send.
type sendParams struct {
	mti            string
	txnType        string
	route          string
	fields         map[int]string
	rrn, stan      string
	maskedPAN      string
	terminalID     string
	requestedAmt   Money
	originalRRN    string
	idempotencyKey string
	requestHash    string
}

// send MACs and sends p.fields, persists the outcome, broadcasts it, and stores the idempotent
// response - the one place every flow's send path runs (Global Constraints: every outgoing
// message is MAC'd, no new path bypasses it).
func (s *Service) send(ctx context.Context, p sendParams) (Transaction, error) {
	if stored, err := s.idempotency.Find(ctx, p.idempotencyKey, p.route); err != nil {
		return Transaction{}, fmt.Errorf("check idempotency: %w", err)
	} else if stored != nil {
		var txn Transaction
		if err := json.Unmarshal(stored.Body, &txn); err != nil {
			return Transaction{}, fmt.Errorf("decode stored transaction: %w", err)
		}
		return txn, nil
	}

	if err := attachMAC(s.hsm, s.zak, p.mti, p.fields); err != nil {
		return Transaction{}, fmt.Errorf("compute outgoing MAC: %w", err)
	}

	row := store.TranLogRow{
		RRN: p.rrn, Type: p.txnType, Status: "SENT", Amount: p.requestedAmt.Amount, Currency: p.requestedAmt.Currency,
		MaskedPAN: p.maskedPAN, TerminalID: p.terminalID, MerchantID: fixedMerchantID, NetworkSTAN: p.stan,
	}
	id, err := s.tranLog.Insert(ctx, row)
	if err != nil {
		return Transaction{}, fmt.Errorf("insert tran_log: %w", err)
	}

	sendCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	resp, err := s.mux.Send(sendCtx, p.mti, p.fields)
	if err != nil {
		return Transaction{}, fmt.Errorf("send %s: %w", p.mti, err)
	}

	txn := mapResponseToTransaction(p.txnType, p.requestedAmt, resp)
	txn.RRN, txn.STAN, txn.MaskedPAN, txn.TerminalID, txn.OriginalRRN = p.rrn, p.stan, p.maskedPAN, p.terminalID, p.originalRRN
	now := time.Now().UTC()
	txn.CreatedAt = now
	txn.BusinessDate = now.Format("2006-01-02")

	if err := s.tranLog.UpdateStatus(ctx, id, txn.Status, txn.ResponseCode, txn.AuthCode); err != nil {
		return Transaction{}, fmt.Errorf("update tran_log: %w", err)
	}

	s.hub.BroadcastTransaction("transaction.created", txn)

	body, err := json.Marshal(txn)
	if err != nil {
		return Transaction{}, fmt.Errorf("encode transaction for idempotency store: %w", err)
	}
	if err := s.idempotency.Store(ctx, p.idempotencyKey, p.route, p.requestHash, 201, body); err != nil {
		return Transaction{}, fmt.Errorf("store idempotent response: %w", err)
	}
	return txn, nil
}

// mapResponseToTransaction reads DE 39 (RC) and DE 4 (amount) from resp - the single shared
// place partial-approval mapping happens (docs/plans/MCN-603.md Task 2: RC 10 sets
// ApprovedAmount, distinct from requestedAmount, not duplicated per flow).
func mapResponseToTransaction(txnType string, requestedAmount Money, resp map[int]string) Transaction {
	txn := Transaction{Type: txnType, Amount: requestedAmount, ResponseCode: resp[39], AuthCode: resp[38]}
	switch txn.ResponseCode {
	case "00":
		txn.Status = "APPROVED"
	case rcPartialApproval:
		txn.Status = "APPROVED"
	default:
		txn.Status = "DECLINED"
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
