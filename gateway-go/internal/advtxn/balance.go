package advtxn

import (
	"context"
	"time"

	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/purchase"
)

const (
	tranTypeBalance = "BALANCE"
	routeBalance    = "balance-inquiries"
)

// CreateBalanceInquiry sends a 0200 with DE 3=310000 (docs/03 "C6 balance inquiry"), no DE 4
// (no amount requested), and parses the response's DE 54 into Transaction.Balance.
func (s *Service) CreateBalanceInquiry(ctx context.Context, req BalanceInquiryRequest, idempotencyKey string) (Transaction, error) {
	return s.idempotent(ctx, routeBalance, idempotencyKey, req, func() (Transaction, error) { return s.createBalanceInquiry(ctx, req) })
}

func (s *Service) createBalanceInquiry(ctx context.Context, req BalanceInquiryRequest) (Transaction, error) {
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
		3:  "310000",
		7:  now.Format("0102150405"),
		11: stan,
		14: card.ExpiryYYMM,
		22: entryModeToDE22[req.EntryMode],
		32: acquirerID,
		37: rrn,
		41: req.TerminalID,
		42: "", // filled by send() from the terminal's merchant
	}

	return s.send(ctx, sendParams{
		mti: "0200", txnType: tranTypeBalance, route: routeBalance, fields: fields,
		rrn: rrn, stan: stan, linkUp: linkUp, sentAt: now, maskedPAN: maskedPAN, terminalID: req.TerminalID,
		cardToken: req.CardToken, posEntryMode: fields[22],
		// No amount is requested, but Money always names a currency (POS-G7).
		requestedAmt: Money{Currency: card.Currency},
	})
}
