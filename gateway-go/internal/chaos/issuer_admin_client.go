package chaos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"go.opentelemetry.io/otel/propagation"
)

const issuerAdminTimeout = 5 * time.Second

// IssuerAdminClient reads card balances from the Issuer Admin API (GET /v1/cards/{cardRef}),
// so a chaos run checks money against the issuer's ledger rather than its own tallies (CHA-G1).
type IssuerAdminClient struct {
	baseURL string
	http    *http.Client
}

// NewIssuerAdminClient builds a client against baseURL (ISSUER_ADMIN_URL, e.g. "http://issuer:8081").
func NewIssuerAdminClient(baseURL string) *IssuerAdminClient {
	return &IssuerAdminClient{baseURL: baseURL, http: &http.Client{Timeout: issuerAdminTimeout}}
}

// LedgerBalance returns the card's ledger balance in minor units and its ISO 4217 currency. The
// W3C traceparent of ctx goes along, so the issuer's log lines join the run's trace.
func (c *IssuerAdminClient) LedgerBalance(ctx context.Context, cardRef string) (int64, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/cards/"+url.PathEscape(cardRef), nil)
	if err != nil {
		return 0, "", err
	}
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(req.Header))
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("issuer admin GET card %s: %w", cardRef, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("issuer admin GET card %s: status %d", cardRef, resp.StatusCode)
	}
	var card struct {
		LedgerBalance *struct {
			Amount   int64  `json:"amount"`
			Currency string `json:"currency"`
		} `json:"ledgerBalance"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return 0, "", fmt.Errorf("issuer admin card %s: decode: %w", cardRef, err)
	}
	if card.LedgerBalance == nil {
		return 0, "", errors.New("issuer admin card " + cardRef + ": no ledgerBalance")
	}
	return card.LedgerBalance.Amount, card.LedgerBalance.Currency, nil
}
