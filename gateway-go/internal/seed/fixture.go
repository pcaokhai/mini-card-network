// Package seed fills the acquirer with a realistic day of transactions for the Overview
// (docs/plans/MCN-002-acquirer-seed.md): it drives real purchases through the gateway, then
// moves their gateway-side timestamps into the windows the dashboard reads.
package seed

import (
	"encoding/json"
	"fmt"
	"os"
)

// Card is the part of a contracts/fixtures/cards.json card the seed needs. The fixture's PAN is
// deliberately not read: the seed only ever addresses cards by token or cardRef.
type Card struct {
	Token   string `json:"cardToken"`
	Ref     string `json:"cardRef"`
	PIN     string `json:"pin"`
	Status  string `json:"status"`
	Balance int64  `json:"balance"`
}

// Terminal is one fixture terminal with the merchant it belongs to.
type Terminal struct {
	TerminalID   string `json:"terminalId"`
	MerchantID   string `json:"merchantId"`
	MerchantName string `json:"merchantName"`
	MCC          string `json:"mcc"`
}

// Fixture is contracts/fixtures/cards.json.
type Fixture struct {
	Cards     []Card     `json:"cards"`
	Terminals []Terminal `json:"terminals"`
}

// LoadFixture reads the fixture at path.
func LoadFixture(path string) (Fixture, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-chosen fixture path for a lab tool
	if err != nil {
		return Fixture{}, fmt.Errorf("read fixture: %w", err)
	}
	var f Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		return Fixture{}, fmt.Errorf("parse fixture: %w", err)
	}
	return f, nil
}

// Card returns the fixture card for token.
func (f Fixture) Card(token string) (Card, bool) {
	for _, c := range f.Cards {
		if c.Token == token {
			return c, true
		}
	}
	return Card{}, false
}

// Terminal returns the fixture terminal with id.
func (f Fixture) Terminal(id string) (Terminal, bool) {
	for _, t := range f.Terminals {
		if t.TerminalID == id {
			return t, true
		}
	}
	return Terminal{}, false
}
