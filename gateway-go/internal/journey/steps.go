package journey

import (
	"fmt"
	"time"
)

const (
	statusSent     = "SENT"
	statusApproved = "APPROVED"
	statusDeclined = "DECLINED"
	statusTimedOut = "TIMED_OUT"
	mtiRequest     = "0200"
)

// requestSteps: the POS request, then either the 0200 going out or a decline without it.
func (b builder) requestSteps() []timedStep {
	start := b.start()
	steps := []timedStep{{start, Step{
		Code: CodePOSRequest, Actor: ActorPOS, Kind: KindOK,
		Title: "POS sends the request", EasyText: "The card was read and the terminal asked for payment.",
		TechnicalText: fmt.Sprintf("POST %s · TID %s · entry mode %s", b.requestPath(), b.txn.TerminalID, orDash(b.txn.POSEntryMode)),
	}}}
	sentAt, sent := b.sentAt()
	if !sent {
		return append(steps, timedStep{start, Step{
			Code: CodeLocalDecline, Actor: ActorAcquirer, Kind: KindBad,
			Title: "Declined before reaching the issuer", EasyText: EasyTextForRC(b.txn.ResponseCode),
			TechnicalText: fmt.Sprintf("Link not signed on · RC %s · no %s sent", b.txn.ResponseCode, b.requestMTI()),
		}})
	}
	return append(steps, timedStep{sentAt, Step{
		Code: CodeRequestSent, Actor: ActorAcquirer, Kind: KindOK,
		Title: "Request sent to the issuer", EasyText: "The acquirer packed the request and sent it to the card's bank.",
		TechnicalText: fmt.Sprintf("%s · STAN %s · RRN %s · amount %d %s", b.requestMTI(), b.txn.NetworkSTAN, b.txn.RRN, b.txn.Amount, b.txn.Currency),
		Message:       b.request(),
	}})
}

func (b builder) sentAt() (time.Time, bool) {
	if at, ok := b.reached(statusSent); ok {
		return at, true
	}
	if b.txn.SentAt != nil {
		return *b.txn.SentAt, true
	}
	return time.Time{}, false
}

// outcomeSteps: what the POS learned, and the reversal queued because of it. A timeout queues
// the reversal before the POS hears back; a cancellation queues it after.
func (b builder) outcomeSteps() []timedStep {
	sentAt, sent := b.sentAt()
	if !sent {
		return []timedStep{{b.start(), b.posResult(statusDeclined, KindBad)}}
	}
	queued := b.reversalQueued()
	if at, ok := b.reached(statusTimedOut); ok {
		steps := append([]timedStep{{at, b.noResponse(msBetween(sentAt, at))}}, queued...)
		return append(steps, timedStep{steps[len(steps)-1].at, b.posResult(statusTimedOut, KindBad)})
	}
	for _, status := range []string{statusApproved, statusDeclined} {
		if at, ok := b.reached(status); ok {
			resp := b.issuerResponse(status, msBetween(sentAt, at))
			return append([]timedStep{{at, resp}, {at, b.posResult(status, resp.Kind)}}, queued...)
		}
	}
	return nil // still waiting for the issuer
}

func (b builder) issuerResponse(status string, elapsedMs int) Step {
	rc := b.txn.ResponseCode
	if status == statusApproved {
		return Step{
			Code: CodeIssuerApproved, Actor: ActorIssuer, Kind: KindOK,
			Title: "Issuer approved", EasyText: b.approvedText(),
			TechnicalText: fmt.Sprintf("%s · RC %s · DE 38 %s · %d ms after the %s", b.responseMTI(), rc, orDash(b.txn.AuthCode), elapsedMs, b.requestMTI()),
			Message:       b.response(),
		}
	}
	return Step{
		Code: CodeIssuerDeclined, Actor: ActorIssuer, Kind: KindBad,
		Title: "Issuer declined", EasyText: EasyTextForRC(rc),
		TechnicalText: fmt.Sprintf("%s · RC %s · %d ms after the %s", b.responseMTI(), rc, elapsedMs, b.requestMTI()),
		Message:       b.response(),
	}
}

func (b builder) noResponse(elapsedMs int) Step {
	return Step{
		Code: CodeNoResponse, Actor: ActorAcquirer, Kind: KindWarn,
		Title:         "No response from the issuer",
		EasyText:      "The card's bank did not answer in time, so the acquirer assumes the money was taken.",
		TechnicalText: fmt.Sprintf("No %s for STAN %s within %d ms · SENT → TIMED_OUT", b.responseMTI(), b.txn.NetworkSTAN, elapsedMs),
	}
}

// posResult shows the status the POS was answered with, not the transaction's current one.
func (b builder) posResult(status string, kind StepKind) Step {
	easy := "The payment went through and the receipt printed."
	if kind != KindOK {
		easy = "The terminal showed the payment failed. No money is lost."
	}
	return Step{
		Code: CodePOSResult, Actor: ActorPOS, Kind: kind,
		Title: "POS shows the result", EasyText: easy,
		TechnicalText: fmt.Sprintf("status %s · RC %s", status, orDash(b.txn.ResponseCode)),
	}
}

func (b builder) reversalQueued() []timedStep {
	at, ok := b.reached(statusReversalPending)
	if !ok {
		return nil
	}
	return []timedStep{{at, Step{
		Code: CodeReversalQueued, Actor: ActorSAF, Kind: KindReversal,
		Title: "Reversal saved before sending", EasyText: "The cancel order was written down first, so it survives a restart.",
		TechnicalText: fmt.Sprintf("INSERT saf_queue (0420, PENDING) · reason %s · state → REVERSAL_PENDING", orDash(b.reversalField(39))),
	}}}
}

// reversalSteps: the 0420 going out (once the worker gave it a STAN) and the issuer's 0430.
func (b builder) reversalSteps() []timedStep {
	var steps []timedStep
	if at, ok := b.reversalSentAt(); ok {
		steps = append(steps, timedStep{at, b.reversalSent()})
	}
	if at, ok := b.reversalConfirmedAt(); ok {
		steps = append(steps, timedStep{at, Step{
			Code: CodeReversalConfirmed, Actor: ActorIssuer, Kind: KindReversal,
			Title: "Issuer confirmed the reversal", EasyText: "The money went back to the cardholder.",
			TechnicalText: fmt.Sprintf("0430 · RC 00 · SAF ACKED after %d send(s) · state → REVERSED", b.reversalSends()),
			Message:       b.reversalAck(),
		}})
	}
	return steps
}

func (b builder) reversalSent() Step {
	mti := b.reversalMTI()
	return Step{
		Code: CodeReversalSent, Actor: ActorAcquirer, Kind: KindReversal,
		Title: "Reversal sent", EasyText: "The cancel order went out, naming the original payment.",
		TechnicalText: fmt.Sprintf("%s · STAN %s · DE 90 → %s STAN %s · attempts %d · SAF %s",
			mti, orDash(b.reversalField(11)), b.requestMTI(), b.txn.NetworkSTAN, b.reversalSends(), b.reversalStatus()),
		Message: b.advice(mti),
	}
}

// reversalSentAt is the advice's DE 7 (its first send). An advice without one was never sent.
// DE 7 is second-precision wire data a seed backdate doesn't rewrite, so it is never placed after
// the 0430 that acknowledged it.
func (b builder) reversalSentAt() (time.Time, bool) {
	if b.rev == nil {
		return b.reached(statusReversed)
	}
	sent, ok := parseDE7(b.rev.Fields[7], b.rev.QueuedAt)
	if ok && b.rev.AckedAt != nil && sent.After(*b.rev.AckedAt) {
		return *b.rev.AckedAt, true
	}
	return sent, ok
}

func (b builder) reversalConfirmedAt() (time.Time, bool) {
	if b.rev != nil && b.rev.AckedAt != nil {
		return *b.rev.AckedAt, true
	}
	return b.reached(statusReversed)
}

// reversalSends counts sends: saf_queue.attempts counts only the failed ones.
func (b builder) reversalSends() int {
	if b.rev == nil {
		return 1
	}
	if b.rev.AckedAt != nil {
		return b.rev.Attempts + 1
	}
	return max(b.rev.Attempts, 1)
}

// reversalMTI is the last send's MTI: every send after the first is an x21 repeat (docs/03 §7.3).
func (b builder) reversalMTI() string {
	if b.reversalSends() > 1 {
		return "0421"
	}
	return "0420"
}

func (b builder) reversalField(de int) string {
	if b.rev == nil {
		return ""
	}
	return b.rev.Fields[de]
}

func (b builder) reversalStatus() string {
	if b.rev == nil {
		return "ACKED"
	}
	return b.rev.Status
}

func (b builder) lateResponse() (Step, bool) {
	if b.txn.LateResponseAt == nil {
		return Step{}, false
	}
	return Step{
		Code: CodeLateResponse, Actor: ActorIssuer, Kind: KindWarn,
		OffsetMs: msBetween(b.start(), *b.txn.LateResponseAt),
		Title:    "Late response", EasyText: "The bank's answer arrived after the acquirer had given up. It is only recorded.",
		TechnicalText: fmt.Sprintf("%s received after timeout, RC %s · state unchanged", b.responseMTI(), b.txn.LateResponseCode),
	}, true
}

// parseDE7 reads an MMDDhhmmss UTC stamp, taking the year from ref.
// ponytail: a first send on 1 Jan for an advice queued on 31 Dec lands a year early; the offset
// clamp pins it to the previous step, harmless for display.
func parseDE7(de7 string, ref time.Time) (time.Time, bool) {
	t, err := time.ParseInLocation("0102150405", de7, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(ref.UTC().Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC), true
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
