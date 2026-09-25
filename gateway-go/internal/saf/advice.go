package saf

import (
	"errors"
	"fmt"
	"time"

	"github.com/mcn/gateway-go/internal/store"
)

const (
	acquirerID       = "970499"
	posConditionCode = "00" // normal presentment
)

var terminalLocation = mustLoadLocation("Asia/Ho_Chi_Minh")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// advice is what saf_queue stores for one reversal: every 0420 field that repeats the original
// request, plus the card token the worker turns into DE 2 at send time. The PAN itself is never
// stored (docs/plans/MCN-401-reversal-0420-fix.md ruling 1).
type advice struct {
	Fields    map[int]string `json:"fields"`
	CardToken string         `json:"cardToken,omitempty"`
}

// errNeverSent means the transaction has no network STAN or DE 7 moment: it never reached the
// issuer, so there is nothing to reverse and DE 90 could not identify it anyway.
var errNeverSent = errors.New("transaction was never sent to the issuer")

// reversalAdvice builds the 0420 fields that repeat original (contracts/iso8583/vectors/
// 0420-reversal-timeout.json). DE 2, 7, 11 and 128 are added per send by the Worker.
func reversalAdvice(original store.TranLogRow, reasonCode string) (advice, error) {
	if original.SentAt == nil || original.NetworkSTAN == "" {
		return advice{}, fmt.Errorf("reverse %s: %w", original.RRN, errNeverSent)
	}
	sent := original.SentAt.UTC()
	local := sent.In(terminalLocation)
	de7 := sent.Format("0102150405")
	return advice{
		CardToken: original.CardToken,
		Fields: map[int]string{
			3:  original.ProcessingCode,
			4:  fmt.Sprintf("%012d", original.Amount),
			12: local.Format("150405"),
			13: local.Format("0102"),
			15: sent.Format("0102"),
			22: original.POSEntryMode,
			25: posConditionCode,
			32: acquirerID,
			37: original.RRN,
			39: reasonCode,
			41: original.TerminalID,
			42: original.MerchantID,
			49: original.Currency,
			// docs/03 §7.3: original MTI + STAN + DE 7 + acquirer ID right-justified zero-filled
			// + forwarding institution (zeros in v1).
			90: mtiPurchase + original.NetworkSTAN + de7 + fmt.Sprintf("%011s", acquirerID) + forwardingInstitutionID,
		},
	}, nil
}
