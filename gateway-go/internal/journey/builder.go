package journey

import (
	"slices"
	"sort"
	"time"

	"github.com/mcn/gateway-go/internal/store"
)

const (
	statusReversalPending = "REVERSAL_PENDING"
	statusReversed        = "REVERSED"
	labelPurchase         = "Purchase"
	labelRefund           = "Refund"
	labelHold             = "Hold"
	labelCompletion       = "Completion"
	labelReversal         = "Reversal"
)

// Actor mirrors contracts/openapi.yaml's Actor enum.
type Actor string

// Actor values.
const (
	ActorPOS      Actor = "POS"
	ActorAcquirer Actor = "ACQUIRER"
	ActorIssuer   Actor = "ISSUER"
	ActorSAF      Actor = "SAF"
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

// StepCode mirrors contracts/openapi.yaml's StepCode enum: what a step is, independent of
// language, so clients render their own copy.
type StepCode string

// StepCode values.
const (
	CodePOSRequest        StepCode = "POS_REQUEST"
	CodeRequestSent       StepCode = "REQUEST_SENT"
	CodeLocalDecline      StepCode = "LOCAL_DECLINE"
	CodeIssuerApproved    StepCode = "ISSUER_APPROVED"
	CodeIssuerDeclined    StepCode = "ISSUER_DECLINED"
	CodeNoResponse        StepCode = "NO_RESPONSE"
	CodeReversalQueued    StepCode = "REVERSAL_QUEUED"
	CodePOSResult         StepCode = "POS_RESULT"
	CodeReversalSent      StepCode = "REVERSAL_SENT"
	CodeReversalConfirmed StepCode = "REVERSAL_CONFIRMED"
	CodeLateResponse      StepCode = "LATE_RESPONSE"
)

// Step mirrors contracts/openapi.yaml's JourneyStep schema. Message is rebuilt from stored
// columns (see message.go); nil on steps that are not an ISO message.
type Step struct {
	Seq           int
	Code          StepCode
	Actor         Actor
	OffsetMs      int
	Title         string
	EasyText      string
	TechnicalText string
	Kind          StepKind
	Message       *IsoMessage
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

// Reversal is a transaction's queued 0420 as saf_queue holds it: the stored advice fields (never
// DE 2) plus the delivery bookkeeping.
type Reversal struct {
	Status   string
	Attempts int // failed sends only (saf.Worker.retryLater)
	QueuedAt time.Time
	AckedAt  *time.Time
	Fields   map[int]string
}

type timedStep struct {
	at   time.Time
	step Step
}

// builder holds what one journey is built from; its step methods live in steps.go and its
// message methods in message.go.
type builder struct {
	txn     store.TranLogRow
	history []store.StateTransition
	rev     *Reversal
}

// BuildJourney turns a transaction's stored columns, tran_state_history and queued reversal (nil
// when none) into the ordered steps of docs/plans/MCN-304-journey-canvas.md.
func BuildJourney(txn store.TranLogRow, history []store.StateTransition, rev *Reversal) Journey {
	b := builder{txn: txn, history: history, rev: rev}
	timed := slices.Concat(b.requestSteps(), b.outcomeSteps(), b.reversalSteps())
	steps := withOffsets(timed, b.start())
	if late, ok := b.lateResponse(); ok {
		steps = insertAtOffset(steps, late)
	}
	for i := range steps {
		steps[i].Seq = i + 1
	}
	return Journey{Steps: steps, Money: moneyRows(txn, steps)}
}

// start is the first event. The request's DE 7 moment (sent_at) is taken before tran_log is
// inserted, so it can precede created_at.
func (b builder) start() time.Time {
	start := b.txn.CreatedAt
	if b.txn.SentAt != nil && (start.IsZero() || b.txn.SentAt.Before(start)) {
		start = *b.txn.SentAt
	}
	if len(b.history) > 0 && (start.IsZero() || b.history[0].At.Before(start)) {
		start = b.history[0].At
	}
	return start
}

// reached returns when the transaction first moved to status.
func (b builder) reached(status string) (time.Time, bool) {
	for _, st := range b.history {
		if st.ToStatus == status {
			return st.At, true
		}
	}
	return time.Time{}, false
}

// withOffsets clamps offsets to be non-decreasing: history and saf_queue timestamps come from
// separate statements, and DE 7 has second precision.
func withOffsets(timed []timedStep, start time.Time) []Step {
	steps := make([]Step, len(timed))
	prev := 0
	for i, ts := range timed {
		ts.step.OffsetMs = max(prev, msBetween(start, ts.at))
		prev = ts.step.OffsetMs
		steps[i] = ts.step
	}
	return steps
}

func msBetween(from, to time.Time) int {
	return max(0, int(to.Sub(from).Milliseconds()))
}

func insertAtOffset(steps []Step, s Step) []Step {
	i := sort.Search(len(steps), func(i int) bool { return steps[i].OffsetMs > s.OffsetMs })
	return slices.Insert(steps, i, s)
}

// moneyRows keeps delta semantics; balanceAfter stays nil because the web reads balances from
// the issuer ledger.
func moneyRows(txn store.TranLogRow, steps []Step) []MoneyRow {
	sign := moneySign(txn.Type)
	debitAt := debitStep(steps)
	if debitAt == 0 || sign == 0 {
		return nil
	}
	rows := []MoneyRow{{Label: moneyLabel(txn.Type), Delta: sign * txn.Amount, AtStep: debitAt}}
	if reversedAt := seqOf(steps, CodeReversalConfirmed); reversedAt > 0 {
		rows = append(rows, MoneyRow{Label: reversalLabel(txn.Type), Delta: -sign * txn.Amount, AtStep: reversedAt})
	}
	return rows
}

// debitStep is where the cardholder's money was (or, on an unknown outcome, is assumed to be)
// held. A plain decline holds nothing; a declined response that still queued a reversal (MAC
// failure) may have been approved at the issuer.
func debitStep(steps []Step) int {
	if seq := seqOf(steps, CodeIssuerApproved); seq > 0 {
		return seq
	}
	if seq := seqOf(steps, CodeNoResponse); seq > 0 {
		return seq
	}
	if seqOf(steps, CodeReversalQueued) > 0 {
		return seqOf(steps, CodeIssuerDeclined)
	}
	return 0
}

func seqOf(steps []Step, code StepCode) int {
	for _, s := range steps {
		if s.Code == code {
			return s.Seq
		}
	}
	return 0
}

func moneyLabel(tranType string) string {
	switch tranType {
	case tranTypeRefund:
		return labelRefund
	case tranTypePreAuth:
		return labelHold
	case tranTypeCompletion:
		return labelCompletion
	default:
		return labelPurchase
	}
}

// reversalLabel names the money a confirmed reversal moves: back to the cardholder, or, for a
// refund, back to the merchant.
func reversalLabel(tranType string) string {
	if tranType == tranTypeRefund {
		return labelReversal
	}
	return labelRefund
}

// LatencyMs is request sent -> issuer response, nil when the issuer never answered (timeout,
// link-down decline, still in flight).
func LatencyMs(txn store.TranLogRow) *int {
	if txn.SentAt == nil || txn.RespondedAt == nil || txn.ResponseCode == "" {
		return nil
	}
	ms := msBetween(*txn.SentAt, *txn.RespondedAt)
	return &ms
}
