package journey

import (
	"fmt"

	"github.com/mcn/gateway-go/internal/store"
)

const (
	statusReversalPending = "REVERSAL_PENDING"
	statusReversed        = "REVERSED"
)

// Actor mirrors contracts/openapi.yaml's Actor enum.
type Actor string

// Actor values.
const (
	ActorPOS    Actor = "POS"
	ActorIssuer Actor = "ISSUER"
	ActorSAF    Actor = "SAF"
)

// StepKind mirrors contracts/openapi.yaml's StepKind enum.
type StepKind string

// StepKind values.
const (
	KindOK       StepKind = "OK"
	KindBad      StepKind = "BAD"
	KindWarn     StepKind = "WARN"
	KindInfo     StepKind = "INFO"
	KindReversal StepKind = "REVERSAL"
)

// Step mirrors contracts/openapi.yaml's JourneyStep schema. Message is left nil: v1 does
// not persist the raw ISO request/response bytes in tran_log (only the summary fields below), so
// there is nothing to embed - a future story that adds message storage would populate it here.
type Step struct {
	Seq           int
	Actor         Actor
	OffsetMs      int
	Title         string
	EasyText      string
	TechnicalText string
	Kind          StepKind
}

// MoneyRow mirrors one row of contracts/openapi.yaml's Journey.money array.
type MoneyRow struct {
	Label        string
	Delta        int64
	BalanceAfter *int64
	AtStep       int
}

// Journey mirrors contracts/openapi.yaml's Journey schema (the Transaction half is built by the
// caller from the same store.TranLogRow; this package only builds Steps and Money).
type Journey struct {
	Steps []Step
	Money []MoneyRow
}

// BuildJourney turns txn's tran_state_history into an ordered Journey. See the Ruling in
// docs/plans/MCN-304.md for why Money has exactly one row (v1's issuer does not report a
// post-transaction balance in DE 54).
func BuildJourney(txn store.TranLogRow, history []store.StateTransition) Journey {
	if len(history) == 0 {
		return Journey{}
	}

	start := history[0].At
	steps := make([]Step, 0, len(history)+1)
	for i, st := range history {
		steps = append(steps, buildStep(i+1, st, int(st.At.Sub(start).Milliseconds()), txn))
	}

	last := steps[len(steps)-1]
	money := []MoneyRow{{
		Label:  moneyLabel(txn.Type),
		Delta:  -txn.Amount,
		AtStep: last.Seq,
	}}
	if txn.Status == statusReversed {
		money = append(money, MoneyRow{Label: "Refund", Delta: txn.Amount, AtStep: last.Seq})
	}

	if txn.LateResponseAt != nil {
		steps = append(steps, Step{
			Seq: len(steps) + 1, Actor: ActorIssuer, OffsetMs: int(txn.LateResponseAt.Sub(start).Milliseconds()), Kind: KindWarn,
			Title:         "Late response",
			EasyText:      "Response arrived too late",
			TechnicalText: fmt.Sprintf("0210 received after timeout, RC %s", txn.LateResponseCode),
		})
	}

	return Journey{Steps: steps, Money: money}
}

func buildStep(seq int, st store.StateTransition, offsetMs int, txn store.TranLogRow) Step {
	actor := actorFor(st.ToStatus)
	kind := kindFor(st.ToStatus)

	switch st.ToStatus {
	case "SENT":
		return Step{
			Seq: seq, Actor: actor, OffsetMs: offsetMs, Kind: kind,
			Title:         "Sent to issuer",
			EasyText:      "Purchase request sent",
			TechnicalText: fmt.Sprintf("0200 sent, RRN %s, amount %d %s", txn.RRN, txn.Amount, txn.Currency),
		}
	case "APPROVED", "DECLINED":
		return Step{
			Seq: seq, Actor: actor, OffsetMs: offsetMs, Kind: kind,
			Title:         "Issuer response",
			EasyText:      EasyTextForRC(txn.ResponseCode),
			TechnicalText: fmt.Sprintf("0210 received, RC %s", txn.ResponseCode),
		}
	case "TIMED_OUT":
		return Step{
			Seq: seq, Actor: actor, OffsetMs: offsetMs, Kind: kind,
			Title:         "No response from issuer",
			EasyText:      "Issuer did not respond in time",
			TechnicalText: "0200 request timed out",
		}
	case statusReversalPending:
		return Step{
			Seq: seq, Actor: actor, OffsetMs: offsetMs, Kind: kind,
			Title:         "Reversal queued",
			EasyText:      "Reversal queued",
			TechnicalText: "0420 enqueued in SAF",
		}
	case statusReversed:
		return Step{
			Seq: seq, Actor: actor, OffsetMs: offsetMs, Kind: kind,
			Title:         "Money returned",
			EasyText:      "Money returned",
			TechnicalText: "0420 delivered, issuer ACKed with 0430",
		}
	default:
		return Step{
			Seq: seq, Actor: actor, OffsetMs: offsetMs, Kind: kind,
			Title:         st.ToStatus,
			EasyText:      st.ToStatus,
			TechnicalText: fmt.Sprintf("%s -> %s", st.FromStatus, st.ToStatus),
		}
	}
}

// actorFor infers who drove a transition per docs/03's message flow: CREATED->SENT is the
// acquirer sending the request, everything after SENT is the issuer's response.
func actorFor(toStatus string) Actor {
	switch toStatus {
	case "SENT", statusReversalPending:
		return ActorPOS
	case statusReversed:
		return ActorSAF
	default:
		return ActorIssuer
	}
}

func kindFor(toStatus string) StepKind {
	switch toStatus {
	case "APPROVED":
		return KindOK
	case "DECLINED", "TIMED_OUT", "FAILED":
		return KindBad
	case statusReversalPending:
		return KindWarn
	case statusReversed:
		return KindReversal
	default:
		return KindInfo
	}
}

func moneyLabel(tranType string) string {
	switch tranType {
	case "REFUND":
		return "Refund"
	default:
		return "Purchase"
	}
}
