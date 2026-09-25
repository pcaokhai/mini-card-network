package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/mcn/gateway-go/internal/store"
)

const (
	currencyVND     = "704"
	limitCardToken  = "tok_limit"
	dailyLimitOnCap = 50_000_000 // above three runs' planned spend on tok_limit
	// A seed whose first purchases all miss their expected outcome is talking to a stack that
	// can't authorise (link down, keys mismatched); stop instead of logging 246 failures.
	earlyAbortAfter = 5
)

// TerminalStore is *store.TerminalRepository.
type TerminalStore interface {
	UpsertFromFixture(ctx context.Context, terminals []store.FixtureTerminal) error
}

// Backdater is *store.TranLogRepository.
type Backdater interface {
	Backdate(ctx context.Context, rrn string, at time.Time) error
}

// Runner executes one seed run against a running stack.
type Runner struct {
	Fixture        Fixture
	GatewayURL     string
	IssuerAdminURL string
	HTTP           *http.Client
	Terminals      TerminalStore
	TranLog        Backdater
	Rand           *rand.Rand
	Now            func() time.Time
	SettleTimeout  time.Duration
	PollInterval   time.Duration
	Logf           func(format string, args ...any)
}

// Summary reports what a run produced.
type Summary struct {
	Planned    int
	Outcomes   map[string]int
	Reversed   int
	Mismatches []string
}

type seeded struct {
	Txn
	rrn string
}

// Run seeds merchants, sets tok_limit's limit, drives every planned purchase through the gateway,
// cancels the planned ones, waits for their reversals, then backdates everything.
func (r *Runner) Run(ctx context.Context) (Summary, error) {
	if err := r.Terminals.UpsertFromFixture(ctx, r.fixtureTerminals()); err != nil {
		return Summary{}, fmt.Errorf("upsert merchants and terminals: %w", err)
	}
	if err := r.setLimit(ctx); err != nil {
		return Summary{}, fmt.Errorf("set %s per-transaction limit: %w", limitCardToken, err)
	}

	planNow := r.Now()
	plan := Build(planNow, r.Rand)
	sum := Summary{Planned: len(plan), Outcomes: map[string]int{}}
	r.Logf("purchasing %d planned transactions", len(plan))
	done, err := r.purchaseAll(ctx, plan, &sum)
	if err != nil {
		return sum, err
	}

	if err := r.cancelPlanned(ctx, done); err != nil {
		return sum, err
	}
	sum.Reversed = r.awaitReversals(ctx, done)

	// Everything is planned relative to planNow; moving by the elapsed time keeps the newest
	// chart bucket full however long the run took.
	shift := r.Now().Sub(planNow)
	for _, s := range done {
		if err := r.TranLog.Backdate(ctx, s.rrn, s.At.Add(shift)); err != nil {
			return sum, fmt.Errorf("backdate %s: %w", s.rrn, err)
		}
	}
	return sum, nil
}

func (r *Runner) fixtureTerminals() []store.FixtureTerminal {
	out := make([]store.FixtureTerminal, len(r.Fixture.Terminals))
	for i, t := range r.Fixture.Terminals {
		out[i] = store.FixtureTerminal{TerminalID: t.TerminalID, MerchantID: t.MerchantID, MerchantName: t.MerchantName, MCC: t.MCC}
	}
	return out
}

func (r *Runner) setLimit(ctx context.Context) error {
	card, ok := r.Fixture.Card(limitCardToken)
	if !ok {
		return fmt.Errorf("fixture has no %s", limitCardToken)
	}
	cardURL := r.IssuerAdminURL + "/v1/cards/" + card.Ref
	status, header, err := r.do(ctx, http.MethodGet, cardURL, nil, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d (is the issuer seeded? run issuer-jpos seed first)", cardURL, status)
	}
	body := map[string]any{
		"perTransactionAmount": map[string]any{"amount": LimitPerTransaction, "currency": currencyVND},
		"dailyAmount":          map[string]any{"amount": dailyLimitOnCap, "currency": currencyVND},
	}
	status, _, err = r.do(ctx, http.MethodPut, cardURL+"/limits", map[string]string{"If-Match": header.Get("ETag")}, body, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("PUT %s/limits: HTTP %d", cardURL, status)
	}
	return nil
}

type purchaseResponse struct {
	RRN          string `json:"rrn"`
	Status       string `json:"status"`
	ResponseCode string `json:"responseCode"`
}

func (r *Runner) purchaseAll(ctx context.Context, plan []Txn, sum *Summary) ([]seeded, error) {
	done := make([]seeded, 0, len(plan))
	for i, txn := range plan {
		got, err := r.purchase(ctx, txn)
		if err != nil {
			return done, fmt.Errorf("purchase %d of %d: %w", i+1, len(plan), err)
		}
		outcome := got.Status
		if got.Status != ExpectApproved {
			outcome = got.ResponseCode
		}
		sum.Outcomes[outcome]++
		if outcome != txn.Expect {
			sum.Mismatches = append(sum.Mismatches, fmt.Sprintf("%s %s %d at %s: want %s, got %s/%s",
				txn.CardToken, txn.TerminalID, txn.Amount, got.RRN, txn.Expect, got.Status, got.ResponseCode))
			if len(sum.Mismatches) == earlyAbortAfter && len(done) < earlyAbortAfter {
				return done, fmt.Errorf("first %d purchases all missed their expected outcome (last: %s); is the gateway signed on and are its keys the issuer's?",
					earlyAbortAfter, sum.Mismatches[len(sum.Mismatches)-1])
			}
		}
		if got.RRN != "" {
			done = append(done, seeded{Txn: txn, rrn: got.RRN})
		}
	}
	return done, nil
}

func (r *Runner) purchase(ctx context.Context, txn Txn) (purchaseResponse, error) {
	body := map[string]any{
		"terminalId": txn.TerminalID,
		"cardToken":  txn.CardToken,
		// No PIN block: the gateway does not forward one yet (risk R-12).
		"entryMode": "CHIP_NO_PIN",
		"amount":    map[string]any{"amount": txn.Amount, "currency": currencyVND},
	}
	var got purchaseResponse
	status, _, err := r.do(ctx, http.MethodPost, r.GatewayURL+"/v1/transactions/purchases", nil, body, &got)
	if err != nil {
		return got, err
	}
	if status != http.StatusCreated {
		return got, fmt.Errorf("POST purchases: HTTP %d", status)
	}
	return got, nil
}

func (r *Runner) cancelPlanned(ctx context.Context, done []seeded) error {
	for _, s := range done {
		if !s.Cancel {
			continue
		}
		status, _, err := r.do(ctx, http.MethodPost, r.GatewayURL+"/v1/transactions/"+s.rrn+"/cancellations", nil, map[string]any{}, nil)
		if err != nil {
			return fmt.Errorf("cancel %s: %w", s.rrn, err)
		}
		if status != http.StatusAccepted {
			return fmt.Errorf("cancel %s: HTTP %d", s.rrn, status)
		}
	}
	return nil
}

// awaitReversals polls each cancelled transaction until the SAF worker's 0420 has been
// acknowledged, returning how many reached REVERSED before SettleTimeout.
func (r *Runner) awaitReversals(ctx context.Context, done []seeded) int {
	pending := map[string]bool{}
	for _, s := range done {
		if s.Cancel {
			pending[s.rrn] = true
		}
	}
	total := len(pending)
	deadline := r.Now().Add(r.SettleTimeout)
	for len(pending) > 0 && r.Now().Before(deadline) {
		r.dropReversed(ctx, pending)
		if len(pending) == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return total - len(pending)
		case <-time.After(r.PollInterval):
		}
	}
	if len(pending) > 0 {
		r.Logf("warning: %d cancellation(s) still not REVERSED after %s", len(pending), r.SettleTimeout)
	}
	return total - len(pending)
}

// dropReversed removes every RRN whose transaction now reads REVERSED.
func (r *Runner) dropReversed(ctx context.Context, pending map[string]bool) {
	for rrn := range pending {
		var got purchaseResponse
		status, _, err := r.do(ctx, http.MethodGet, r.GatewayURL+"/v1/transactions/"+rrn, nil, nil, &got)
		if err == nil && status == http.StatusOK && got.Status == "REVERSED" {
			delete(pending, rrn)
		}
	}
}

// do sends a JSON request; state-changing calls carry a fresh Idempotency-Key (docs/04 §2).
func (r *Runner) do(ctx context.Context, method, url string, headers map[string]string, body, out any) (int, http.Header, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.NewString())
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, resp.Header, fmt.Errorf("decode %s %s: %w", method, url, err)
		}
	}
	return resp.StatusCode, resp.Header, nil
}
