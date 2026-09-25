package advtxn

import (
	"context"
	"fmt"
	"time"

	"github.com/mcn/gateway-go/internal/purchase"
)

const (
	tranTypeCompletion = "COMPLETION"
	routeCompletion    = "completions"
)

// CreateCompletion sends a 0220 completing the pre-authorization identified by rrn, DE 37
// referencing it (docs/03 "C8 completion references the pre-auth").
func (s *Service) CreateCompletion(ctx context.Context, rrn string, req CompletionRequest, idempotencyKey string) (Transaction, error) {
	// The completion happens at the pre-auth's terminal and card; the request carries neither.
	preAuth, err := s.tranLog.Get(ctx, rrn)
	if err != nil {
		return Transaction{}, fmt.Errorf("look up pre-authorization %s: %w", rrn, err)
	}
	stan, linkUp := s.nextSTAN()
	now := time.Now().UTC()
	completionRRN := purchase.BuildRRN(now, stan)

	fields := map[int]string{
		3:  "000000",
		4:  fmt.Sprintf("%012d", req.Amount.Amount),
		7:  now.Format("0102150405"),
		11: stan,
		32: acquirerID,
		37: rrn,
		49: req.Amount.Currency,
	}

	return s.send(ctx, sendParams{
		mti: "0220", txnType: tranTypeCompletion, route: routeCompletion, fields: fields,
		rrn: completionRRN, stan: stan, linkUp: linkUp, sentAt: now, originalRRN: rrn,
		terminalID: preAuth.TerminalID, maskedPAN: preAuth.MaskedPAN,
		cardToken: preAuth.CardToken, posEntryMode: preAuth.POSEntryMode,
		requestedAmt: req.Amount, idempotencyKey: idempotencyKey, requestHash: hashRequest(req),
	})
}
