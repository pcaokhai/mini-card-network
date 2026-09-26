package advtxn

import (
	"context"
	"fmt"
	"time"

	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/purchase"
)

const (
	tranTypePreAuth = "PREAUTH"
	routePreAuth    = "pre-authorizations"
)

// entryModeToDE22 mirrors contracts/openapi.yaml's EntryMode enum, same mapping
// purchase.Service uses (docs/03 §3).
var entryModeToDE22 = map[string]string{
	"CHIP_PIN":      "051",
	"CHIP_NO_PIN":   "052",
	"MANUAL_PIN":    "011",
	"MANUAL_NO_PIN": "012",
}

// ErrUnknownCardToken is returned when a request names a cardToken absent from
// contracts/fixtures/cards.json.
var ErrUnknownCardToken = purchase.ErrUnknownCardToken

// CreatePreAuth sends a 0100 with DE 3=000000, DE 25=06 (pre-authorization hold, docs/03 §7.3).
func (s *Service) CreatePreAuth(ctx context.Context, req PreAuthRequest, idempotencyKey string) (Transaction, error) {
	return s.idempotent(ctx, routePreAuth, idempotencyKey, req, func(ctx context.Context) (Transaction, error) { return s.createPreAuth(ctx, req) })
}

func (s *Service) createPreAuth(ctx context.Context, req PreAuthRequest) (Transaction, error) {
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
		3:  "000000",
		4:  fmt.Sprintf("%012d", req.Amount.Amount),
		7:  now.Format("0102150405"),
		11: stan,
		14: card.ExpiryYYMM,
		22: entryModeToDE22[req.EntryMode],
		25: "06",
		32: acquirerID,
		37: rrn,
		41: req.TerminalID,
		42: "", // filled by send() from the terminal's merchant
		49: req.Amount.Currency,
	}

	return s.send(ctx, sendParams{
		mti: "0100", txnType: tranTypePreAuth, route: routePreAuth, fields: fields,
		rrn: rrn, stan: stan, linkUp: linkUp, sentAt: now, maskedPAN: maskedPAN, terminalID: req.TerminalID,
		cardToken: req.CardToken, posEntryMode: fields[22],
		requestedAmt: req.Amount,
	})
}
