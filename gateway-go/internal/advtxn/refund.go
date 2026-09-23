package advtxn

import (
	"context"
	"fmt"
	"time"

	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/purchase"
)

const (
	tranTypeRefund = "REFUND"
	routeRefund    = "refunds"
)

// CreateRefund sends a 0200 with DE 3=200000 (docs/03 §6 refund processing code).
func (s *Service) CreateRefund(ctx context.Context, req RefundRequest, idempotencyKey string) (Transaction, error) {
	card, ok := s.cardTokens.Resolve(req.CardToken)
	if !ok {
		return Transaction{}, ErrUnknownCardToken
	}
	maskedPAN := obs.MaskPAN(card.PAN)

	stan, ok := s.mux.NextSTAN()
	if !ok {
		return Transaction{}, fmt.Errorf("advtxn: no STAN available")
	}
	now := time.Now().UTC()
	rrn := purchase.BuildRRN(now, stan)

	fields := map[int]string{
		2:  card.PAN,
		3:  "200000",
		4:  fmt.Sprintf("%012d", req.Amount.Amount),
		7:  now.Format("0102150405"),
		11: stan,
		14: card.ExpiryYYMM,
		22: entryModeToDE22[req.EntryMode],
		32: acquirerID,
		37: rrn,
		41: req.TerminalID,
		42: fixedMerchantID,
		49: req.Amount.Currency,
	}

	return s.send(ctx, sendParams{
		mti: "0200", txnType: tranTypeRefund, route: routeRefund, fields: fields,
		rrn: rrn, stan: stan, maskedPAN: maskedPAN, terminalID: req.TerminalID,
		requestedAmt: req.Amount, idempotencyKey: idempotencyKey, requestHash: hashRequest(req),
	})
}
