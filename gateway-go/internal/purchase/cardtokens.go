// Package purchase builds and sends purchase (0200) transactions through the acquirer's live
// issuer link and persists their outcome. See cardtokens.go's Ruling for the cardToken->PAN
// design, and docs/plans/MCN-303.md for the full story context.
package purchase

//go:generate go run ./gen

// CardFixture is one contracts/fixtures/cards.json test card's PAN and expiry, resolved from a
// simulator cardToken. The real PAN never crosses the wire beyond DE 2 of the outbound 0200 and
// is never logged, persisted, or returned — only its masked form (obs.MaskPAN) is.
type CardFixture struct {
	PAN        string
	ExpiryYYMM string
}

// CardTokenRegistry resolves a simulator cardToken to its CardFixture.
type CardTokenRegistry struct {
	byToken map[string]CardFixture
}

// Resolve looks up token, returning ok=false for an unknown token.
func (r *CardTokenRegistry) Resolve(token string) (CardFixture, bool) {
	c, ok := r.byToken[token]
	return c, ok
}

// DefaultCardTokens returns the registry generated from contracts/fixtures/cards.json.
func DefaultCardTokens() *CardTokenRegistry {
	return &CardTokenRegistry{byToken: defaultCards}
}
