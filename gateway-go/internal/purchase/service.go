package purchase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "time/tzdata" // Asia/Ho_Chi_Minh must resolve even on a minimal container image

	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	acquirerID     = "970499" // docs/03 §3: Lab acquirer institution ID (DE 32)
	purchaseRoute  = "purchases"
	requestTimeout = 30 * time.Second // docs/03 §9
)

// ponytail: v1 seeds exactly one terminal/merchant (contracts/fixtures/cards.json's terminals
// array, migrations/00002_transactions.sql); add a terminal/merchant lookup port when a second
// one is seeded.
const (
	fixedMerchantID   = "GOCPHO000000001"
	fixedMerchantName = "Ca phe Goc Pho"
)

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
	UpdateStatus(ctx context.Context, id int64, status string) error
	RecordStateTransition(ctx context.Context, id int64, fromStatus, toStatus string) error
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
}

// Service builds and sends purchase transactions.
type Service struct {
	mux         MuxSender
	linkStatus  LinkStatusPort
	cardTokens  *CardTokenRegistry
	tranLog     TranLogPort
	idempotency IdempotencyPort
	hub         HubPort
}

// NewService builds a Service. mux also serves as the LinkStatusPort (e.g. *isonet.Supervisor
// implements both).
func NewService(mux interface {
	MuxSender
	LinkStatusPort
}, cardTokens *CardTokenRegistry, tranLog TranLogPort, idempotency IdempotencyPort, hub HubPort) *Service {
	return &Service{mux: mux, linkStatus: mux, cardTokens: cardTokens, tranLog: tranLog, idempotency: idempotency, hub: hub}
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

	card, ok := s.cardTokens.Resolve(req.CardToken)
	if !ok {
		return Transaction{}, ErrUnknownCardToken
	}
	maskedPAN := obs.MaskPAN(card.PAN)

	if !s.linkStatus.IsSignedOn() {
		txn := s.declinedTransaction(req, maskedPAN, "91")
		if err := s.persistAndBroadcast(ctx, txn, req, idempotencyKey, requestHash); err != nil {
			return Transaction{}, err
		}
		return txn, nil
	}

	stan, ok := s.mux.NextSTAN()
	if !ok {
		// Link went down between the check above and now; same outcome as the check failing.
		txn := s.declinedTransaction(req, maskedPAN, "91")
		if err := s.persistAndBroadcast(ctx, txn, req, idempotencyKey, requestHash); err != nil {
			return Transaction{}, err
		}
		return txn, nil
	}

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
		42: fixedMerchantID,
		49: req.Amount.Currency,
	}

	row := store.TranLogRow{
		RRN: rrn, Type: "PURCHASE", Status: "CREATED", Amount: req.Amount.Amount, Currency: req.Amount.Currency,
		MaskedPAN: maskedPAN, TerminalID: req.TerminalID, MerchantID: fixedMerchantID, NetworkSTAN: stan,
	}
	id, err := s.tranLog.Insert(ctx, row)
	if err != nil {
		return Transaction{}, fmt.Errorf("insert tran_log: %w", err)
	}
	if err := s.tranLog.UpdateStatus(ctx, id, "SENT"); err != nil {
		return Transaction{}, fmt.Errorf("update tran_log to SENT: %w", err)
	}
	if err := s.tranLog.RecordStateTransition(ctx, id, "CREATED", "SENT"); err != nil {
		return Transaction{}, fmt.Errorf("record CREATED->SENT: %w", err)
	}

	sendCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	resp, err := s.mux.Send(sendCtx, "0200", fields)

	var txn Transaction
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		// Known follow-up for MCN-401 (reversal/SAF): no reversal is queued here yet (YAGNI -
		// that mechanism doesn't exist until MCN-401).
		txn = s.baseTransaction(req, rrn, stan, maskedPAN)
		txn.Status = "TIMED_OUT"
		if uErr := s.tranLog.UpdateStatus(ctx, id, "TIMED_OUT"); uErr != nil {
			return Transaction{}, fmt.Errorf("update tran_log to TIMED_OUT: %w", uErr)
		}
		_ = s.tranLog.RecordStateTransition(ctx, id, "SENT", "TIMED_OUT")
	case err != nil:
		return Transaction{}, fmt.Errorf("send purchase: %w", err)
	default:
		rc := resp[39]
		txn = s.baseTransaction(req, rrn, stan, maskedPAN)
		txn.ResponseCode = rc
		txn.AuthCode = resp[38]
		if rc == "00" {
			txn.Status = "APPROVED"
		} else {
			txn.Status = "DECLINED"
		}
		if uErr := s.tranLog.UpdateStatus(ctx, id, txn.Status); uErr != nil {
			return Transaction{}, fmt.Errorf("update tran_log to %s: %w", txn.Status, uErr)
		}
		_ = s.tranLog.RecordStateTransition(ctx, id, "SENT", txn.Status)
	}

	if err := s.persistAndBroadcast(ctx, txn, req, idempotencyKey, requestHash); err != nil {
		return Transaction{}, err
	}
	return txn, nil
}

func (s *Service) baseTransaction(req PurchaseRequest, rrn, stan, maskedPAN string) Transaction {
	now := time.Now().UTC()
	return Transaction{
		RRN: rrn, STAN: stan, Type: "PURCHASE", Amount: req.Amount, MaskedPAN: maskedPAN,
		TerminalID: req.TerminalID, MerchantName: fixedMerchantName, CreatedAt: now,
		BusinessDate: now.Format("2006-01-02"),
	}
}

// declinedTransaction builds a decline for a purchase that was never sent (no STAN was ever
// allocated, since mux.Send is never called on this path). Its RRN uses STAN "000000" per
// docs/03 §5's format - ponytail: collisions are possible if two link-down declines land in the
// same UTC second, acceptable for this lab; a real deployment would reserve a STAN even for
// link-down declines.
func (s *Service) declinedTransaction(req PurchaseRequest, maskedPAN, responseCode string) Transaction {
	rrn := BuildRRN(time.Now().UTC(), "000000")
	txn := s.baseTransaction(req, rrn, "000000", maskedPAN)
	txn.Status = "DECLINED"
	txn.ResponseCode = responseCode
	return txn
}

// persistAndBroadcast records a link-down decline in tran_log, broadcasts the transaction, and
// stores the idempotent response so a replay never resends.
func (s *Service) persistAndBroadcast(ctx context.Context, txn Transaction, req PurchaseRequest, idempotencyKey, requestHash string) error {
	if txn.Status == "DECLINED" && txn.ResponseCode == "91" {
		// Link-down decline: mux.Send was never called, so this row wasn't logged earlier in
		// CreatePurchase; log it now so it's still visible in tran_log.
		row := store.TranLogRow{
			RRN: txn.RRN, Type: "PURCHASE", Status: "DECLINED",
			Amount: req.Amount.Amount, Currency: req.Amount.Currency, MaskedPAN: txn.MaskedPAN,
			TerminalID: req.TerminalID, MerchantID: fixedMerchantID, ResponseCode: txn.ResponseCode,
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

func hashRequest(req PurchaseRequest) string {
	body, _ := json.Marshal(req)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
