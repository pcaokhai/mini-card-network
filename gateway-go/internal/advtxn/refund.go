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
	return s.idempotent(ctx, routeRefund, idempotencyKey, req, func() (Transaction, error) { return s.createRefund(ctx, req) })
}

func (s *Service) createRefund(ctx context.Context, req RefundRequest) (Transaction, error) {
	card, ok := s.cardTokens.Resolve(req.CardToken)
	if !ok {
		return Transaction{}, ErrUnknownCardToken
	}
	maskedPAN := obs.MaskPAN(card.PAN)

	stan, linkUp := s.nextSTAN()
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
		42: "", // filled by send() from the terminal's merchant
		49: req.Amount.Currency,
	}

	return s.send(ctx, sendParams{
		mti: "0200", txnType: tranTypeRefund, route: routeRefund, fields: fields,
		rrn: rrn, stan: stan, linkUp: linkUp, sentAt: now, maskedPAN: maskedPAN, terminalID: req.TerminalID,
		cardToken: req.CardToken, posEntryMode: fields[22],
		requestedAmt: req.Amount,
	})
}
