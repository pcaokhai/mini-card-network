package journey

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

const (
	statusCreated = "CREATED"
	maskedPAN     = "970436******4417"
	testSTAN      = "000124"
	testRRN       = "626514000124"
	adviceSTAN    = "000125"
	testDE7       = "0921073244"
	purchase      = "PURCHASE"
	vnd           = "704"
	rcApproved    = "00"
	testAuthCode  = "A00124"
	rcTimeout     = "68"
)

var t0 = time.Date(2026, 9, 21, 7, 32, 44, 0, time.UTC)

func at(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

func sentRow(status, rc, auth string) store.TranLogRow {
	sent := t0
	return store.TranLogRow{
		ID: 1, RRN: testRRN, Type: purchase, Status: status, Amount: 600000, Currency: vnd,
		MaskedPAN: maskedPAN, TerminalID: "00000042", MerchantID: "GOCPHO000000001",
		NetworkSTAN: testSTAN, ResponseCode: rc, AuthCode: auth, CreatedAt: at(2),
		ProcessingCode: "000000", POSEntryMode: "051", SentAt: &sent,
	}
}

func tr(from, to string, ms int) store.StateTransition {
	return store.StateTransition{FromStatus: from, ToStatus: to, At: at(ms)}
}

func storedAdvice(reason string) map[int]string {
	return map[int]string{
		3: "000000", 4: "000000600000", 7: "0921073315", 11: adviceSTAN, 37: testRRN, 39: reason,
		41: "00000042", 42: "GOCPHO000000001", 49: vnd, 90: mtiRequest + testSTAN + testDE7 + "00000970499" + "00000000000",
	}
}

func ackedReversal(reason string, attempts int, ackMs int) *Reversal {
	acked := at(ackMs)
	return &Reversal{Status: "ACKED", Attempts: attempts, QueuedAt: at(30010), AckedAt: &acked, Fields: storedAdvice(reason)}
}

func approvedHistory() []store.StateTransition {
	return []store.StateTransition{tr(statusCreated, statusSent, 6), tr(statusSent, statusApproved, 168)}
}

func declinedHistory() []store.StateTransition {
	return []store.StateTransition{tr(statusCreated, statusSent, 6), tr(statusSent, statusDeclined, 90)}
}

func timeoutHistory(reversedMs int) []store.StateTransition {
	return []store.StateTransition{
		tr(statusCreated, statusSent, 8), tr(statusSent, statusTimedOut, 30000),
		tr(statusTimedOut, statusReversalPending, 30010), tr(statusReversalPending, statusReversed, reversedMs),
	}
}

func codes(j Journey) []StepCode {
	out := make([]StepCode, len(j.Steps))
	for i, s := range j.Steps {
		out[i] = s.Code
	}
	return out
}

func TestBuildJourney_sequencesByOutcome__MCN_304(t *testing.T) {
	linkDown := store.TranLogRow{RRN: "626514000000", Type: purchase, Status: statusDeclined, ResponseCode: "91", Amount: 100, Currency: vnd, MaskedPAN: maskedPAN, CreatedAt: t0}
	tests := []struct {
		name    string
		txn     store.TranLogRow
		history []store.StateTransition
		rev     *Reversal
		want    []StepCode
		actors  []Actor
		kinds   []StepKind
	}{
		{
			name: "approved", txn: sentRow(statusApproved, rcApproved, testAuthCode), history: approvedHistory(),
			want:   []StepCode{CodePOSRequest, CodeRequestSent, CodeIssuerApproved, CodePOSResult},
			actors: []Actor{ActorPOS, ActorAcquirer, ActorIssuer, ActorPOS},
			kinds:  []StepKind{KindOK, KindOK, KindOK, KindOK},
		},
		{
			name: "declined", txn: sentRow(statusDeclined, "51", ""), history: declinedHistory(),
			want:   []StepCode{CodePOSRequest, CodeRequestSent, CodeIssuerDeclined, CodePOSResult},
			actors: []Actor{ActorPOS, ActorAcquirer, ActorIssuer, ActorPOS},
			kinds:  []StepKind{KindOK, KindOK, KindBad, KindBad},
		},
		{
			name: "link-down decline", txn: linkDown,
			want:   []StepCode{CodePOSRequest, CodeLocalDecline, CodePOSResult},
			actors: []Actor{ActorPOS, ActorAcquirer, ActorPOS},
			kinds:  []StepKind{KindOK, KindBad, KindBad},
		},
		{
			name: "still in flight", txn: sentRow(statusSent, "", ""), history: []store.StateTransition{tr(statusCreated, statusSent, 6)},
			want:   []StepCode{CodePOSRequest, CodeRequestSent},
			actors: []Actor{ActorPOS, ActorAcquirer},
			kinds:  []StepKind{KindOK, KindOK},
		},
		{
			name: "timeout reversed", txn: sentRow(statusReversed, "", ""), history: timeoutHistory(30090), rev: ackedReversal(rcTimeout, 0, 30090),
			want:   []StepCode{CodePOSRequest, CodeRequestSent, CodeNoResponse, CodeReversalQueued, CodePOSResult, CodeReversalSent, CodeReversalConfirmed},
			actors: []Actor{ActorPOS, ActorAcquirer, ActorAcquirer, ActorSAF, ActorPOS, ActorAcquirer, ActorIssuer},
			kinds:  []StepKind{KindOK, KindOK, KindWarn, KindReversal, KindBad, KindReversal, KindReversal},
		},
		{
			name: "timeout, reversal queued but never sent", txn: sentRow(statusReversalPending, "", ""), history: timeoutHistory(0)[:3],
			rev:    &Reversal{Status: "PENDING", QueuedAt: at(30010), Fields: map[int]string{39: rcTimeout}},
			want:   []StepCode{CodePOSRequest, CodeRequestSent, CodeNoResponse, CodeReversalQueued, CodePOSResult},
			actors: []Actor{ActorPOS, ActorAcquirer, ActorAcquirer, ActorSAF, ActorPOS},
			kinds:  []StepKind{KindOK, KindOK, KindWarn, KindReversal, KindBad},
		},
		{
			name: "cancellation", txn: sentRow(statusReversed, rcApproved, testAuthCode),
			history: append(approvedHistory(), tr(statusApproved, statusReversalPending, 5000), tr(statusReversalPending, statusReversed, 5100)),
			rev:     ackedReversal("17", 0, 5100),
			want:    []StepCode{CodePOSRequest, CodeRequestSent, CodeIssuerApproved, CodePOSResult, CodeReversalQueued, CodeReversalSent, CodeReversalConfirmed},
			actors:  []Actor{ActorPOS, ActorAcquirer, ActorIssuer, ActorPOS, ActorSAF, ActorAcquirer, ActorIssuer},
			kinds:   []StepKind{KindOK, KindOK, KindOK, KindOK, KindReversal, KindReversal, KindReversal},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			j := BuildJourney(tc.txn, tc.history, tc.rev)

			require.Equal(t, tc.want, codes(j))
			for i, s := range j.Steps {
				require.Equal(t, i+1, s.Seq)
				require.Equal(t, tc.actors[i], s.Actor, "actor of %s", s.Code)
				require.Equal(t, tc.kinds[i], s.Kind, "kind of %s", s.Code)
				require.NotEmpty(t, s.Title)
				require.NotEmpty(t, s.EasyText)
				require.NotEmpty(t, s.TechnicalText)
			}
		})
	}
}

func TestBuildJourney_offsetsAreMonotonicFromFirstEvent__MCN_304(t *testing.T) {
	rev := ackedReversal(rcTimeout, 0, 30090)
	rev.Fields[7] = "0921073314" // second precision: before the queue time, must clamp

	j := BuildJourney(sentRow(statusReversed, "", ""), timeoutHistory(30090), rev)

	require.Equal(t, 0, j.Steps[0].OffsetMs)
	require.Equal(t, 8, j.Steps[1].OffsetMs)
	require.Equal(t, 30000, j.Steps[2].OffsetMs)
	for i := 1; i < len(j.Steps); i++ {
		require.GreaterOrEqual(t, j.Steps[i].OffsetMs, j.Steps[i-1].OffsetMs, "step %d", i+1)
	}
	require.Equal(t, 30090, j.Steps[len(j.Steps)-1].OffsetMs)
}

func TestBuildJourney_lateResponseLandsAtItsOwnOffset__MCN_304(t *testing.T) {
	txn := sentRow(statusReversed, "", "")
	late := at(33400)
	txn.LateResponseAt, txn.LateResponseCode = &late, rcApproved

	j := BuildJourney(txn, timeoutHistory(36300), ackedReversal(rcTimeout, 2, 36300))

	require.Equal(t, []StepCode{CodePOSRequest, CodeRequestSent, CodeNoResponse, CodeReversalQueued, CodePOSResult, CodeReversalSent, CodeLateResponse, CodeReversalConfirmed}, codes(j))
	lateStep := j.Steps[6]
	require.Equal(t, 33400, lateStep.OffsetMs)
	require.Equal(t, 7, lateStep.Seq)
	require.Equal(t, ActorIssuer, lateStep.Actor)
	require.Equal(t, KindWarn, lateStep.Kind)
	require.Contains(t, lateStep.TechnicalText, "RC 00")
}

func TestBuildJourney_reversalSentIs0421AfterFailedAttempts__MCN_304(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		wantMTI  string
		wantText string
	}{
		{"first send ACKed", 0, "0420", "attempts 1"},
		{"ACKed on the third send", 2, "0421", "attempts 3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			j := BuildJourney(sentRow(statusReversed, "", ""), timeoutHistory(36300), ackedReversal(rcTimeout, tc.attempts, 36300))

			sent := j.Steps[5]
			require.Equal(t, CodeReversalSent, sent.Code)
			require.Equal(t, tc.wantMTI, sent.Message.MTI)
			require.Contains(t, sent.TechnicalText, tc.wantText)
			require.Contains(t, sent.TechnicalText, "STAN "+adviceSTAN)
		})
	}
}

func fieldDEs(m *IsoMessage) []string {
	out := make([]string, len(m.Fields))
	for i, f := range m.Fields {
		out[i] = f.DE
	}
	return out
}

func fieldValue(m *IsoMessage, de string) string {
	for _, f := range m.Fields {
		if f.DE == de {
			return f.Value
		}
	}
	return ""
}

func TestBuildJourney_messagesCarryCanvasFieldSets__MCN_304(t *testing.T) {
	approved := BuildJourney(sentRow(statusApproved, rcApproved, testAuthCode), approvedHistory(), nil)
	reversed := BuildJourney(sentRow(statusReversed, "", ""), timeoutHistory(30090), ackedReversal(rcTimeout, 0, 30090))

	req, resp := approved.Steps[1].Message, approved.Steps[2].Message
	adv, ack := reversed.Steps[5].Message, reversed.Steps[6].Message

	require.Nil(t, approved.Steps[0].Message)
	require.Nil(t, approved.Steps[3].Message)
	require.Equal(t, []string{deMTI, "2", "3", "4", "7", "11", "22", "37", "41", "42", "49"}, fieldDEs(req))
	require.Equal(t, []string{deMTI, "2", "3", "4", "7", "11", "37", "38", "39", "41", "42", "49"}, fieldDEs(resp))
	require.Equal(t, []string{deMTI, "2", "3", "4", "7", "11", "37", "39", "41", "42", "49", "90"}, fieldDEs(adv))
	require.Equal(t, []string{deMTI, "2", "3", "4", "7", "11", "37", "39", "41", "42", "49"}, fieldDEs(ack))

	require.Equal(t, mtiRequest, fieldValue(req, deMTI))
	require.Equal(t, "000000600000", fieldValue(req, "4"))
	require.Equal(t, testDE7, fieldValue(req, "7"))
	require.Equal(t, testSTAN, fieldValue(req, "11"))
	require.Equal(t, testAuthCode, fieldValue(resp, "38"))
	require.Equal(t, rcApproved, fieldValue(resp, "39"))
	require.Equal(t, rcTimeout, fieldValue(adv, "39"))
	require.Equal(t, adviceSTAN, fieldValue(adv, "11"))
	require.Equal(t, mtiRequest+testSTAN+testDE7+"00000970499"+"00000000000", fieldValue(adv, "90"))
	require.Equal(t, "0430", ack.MTI)
	require.Equal(t, rcApproved, fieldValue(ack, "39"))
	for _, f := range append(req.Fields, adv.Fields...) {
		require.NotEmpty(t, f.EasyName, f.DE)
		require.NotEmpty(t, f.TechnicalName, f.DE)
		require.NotEmpty(t, f.Format, f.DE)
	}
}

func TestBuildJourney_declinedResponseHasNoDE38__MCN_304(t *testing.T) {
	j := BuildJourney(sentRow(statusDeclined, "51", ""), declinedHistory(), nil)

	require.NotContains(t, fieldDEs(j.Steps[2].Message), "38")
	require.Equal(t, "51", fieldValue(j.Steps[2].Message, "39"))
}

var clearPAN = regexp.MustCompile(`\d{13,19}`)

func TestBuildJourney_de2IsMaskedPanOnly__MCN_304(t *testing.T) {
	for _, stored := range []string{maskedPAN, "9704360000004417"} { // the second must never leak
		t.Run(stored, func(t *testing.T) {
			txn := sentRow(statusApproved, rcApproved, testAuthCode)
			txn.MaskedPAN = stored

			j := BuildJourney(txn, approvedHistory(), nil)

			for _, s := range j.Steps {
				if s.Message == nil {
					continue
				}
				require.Equal(t, maskedPAN, fieldValue(s.Message, "2"))
				require.False(t, clearPAN.MatchString(fieldValue(s.Message, "2")), "clear PAN in DE 2")
			}
		})
	}
}

func TestBuildJourney_moneyDeltas__MCN_304(t *testing.T) {
	approved := BuildJourney(sentRow(statusApproved, rcApproved, testAuthCode), approvedHistory(), nil)
	declined := BuildJourney(sentRow(statusDeclined, "51", ""), declinedHistory(), nil)
	reversed := BuildJourney(sentRow(statusReversed, "", ""), timeoutHistory(30090), ackedReversal(rcTimeout, 0, 30090))

	require.Equal(t, []MoneyRow{{Label: labelPurchase, Delta: -600000, AtStep: 3}}, approved.Money)
	require.Empty(t, declined.Money)
	require.Equal(t, []MoneyRow{
		{Label: labelPurchase, Delta: -600000, AtStep: 3},
		{Label: labelRefund, Delta: 600000, AtStep: 7},
	}, reversed.Money)
}

func TestLatencyMs__MCN_304(t *testing.T) {
	responded := at(168)
	answered := sentRow(statusApproved, rcApproved, testAuthCode)
	answered.RespondedAt = &responded
	timedOut := sentRow(statusTimedOut, "", "")
	timedOut.RespondedAt = &responded

	require.Equal(t, 168, *LatencyMs(answered))
	require.Nil(t, LatencyMs(timedOut))
	require.Nil(t, LatencyMs(store.TranLogRow{ResponseCode: "91"}))
}

func TestEasyTextForRC_coversEveryDocumentedCode(t *testing.T) {
	for _, rc := range []string{"00", "05", "06", "10", "12", "13", "14", "17", "30", "51", "54", "55", "57", "61", "62", "65", "68", "75", "91", "94", "95", "96"} {
		require.NotEmpty(t, EasyTextForRC(rc), "missing easy text for RC %s", rc)
	}
}

func TestBuildJourney_posResultShowsWhatThePOSSaw__MCN_304(t *testing.T) {
	cancelled := BuildJourney(sentRow(statusReversed, rcApproved, testAuthCode),
		append(approvedHistory(), tr(statusApproved, statusReversalPending, 5000), tr(statusReversalPending, statusReversed, 5100)), ackedReversal("17", 0, 5100))
	timedOut := BuildJourney(sentRow(statusReversed, "", ""), timeoutHistory(30090), ackedReversal(rcTimeout, 0, 30090))

	require.Contains(t, cancelled.Steps[3].TechnicalText, "status APPROVED")
	require.Contains(t, timedOut.Steps[4].TechnicalText, "status TIMED_OUT")
}

// DE 7 is second-precision wire data that a seed backdate can't rewrite, so it can land after the
// issuer's acknowledgement; the send is never shown after its own 0430.
func TestBuildJourney_reversalSentNeverAfterItsAcknowledgement__MCN_002(t *testing.T) {
	rev := ackedReversal(rcTimeout, 0, 30090)
	rev.Fields[7] = at(30090 + 965_000).Format("0102150405")

	j := BuildJourney(sentRow(statusReversed, "", ""), timeoutHistory(30090), rev)

	sent, confirmed := j.Steps[5], j.Steps[6]
	require.Equal(t, CodeReversalSent, sent.Code)
	require.Equal(t, CodeReversalConfirmed, confirmed.Code)
	require.Equal(t, confirmed.OffsetMs, sent.OffsetMs)
	require.Less(t, confirmed.OffsetMs, 31_000)
}
