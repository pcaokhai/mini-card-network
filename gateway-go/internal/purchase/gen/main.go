// Command gen reads contracts/fixtures/cards.json and writes cards_gen.go.
// Run via `go generate ./...` from gateway-go/. Never edit cards_gen.go by hand.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"log"
	"os"
)

type rawCard struct {
	CardToken string `json:"cardToken"`
	PAN       string `json:"pan"`
	Expiry    string `json:"expiry"`
}

type rawFixture struct {
	Cards []rawCard `json:"cards"`
}

func main() {
	data, err := os.ReadFile("../../../contracts/fixtures/cards.json")
	if err != nil {
		log.Fatal(err)
	}
	var fixture rawFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	fmt.Fprintln(&buf, "// Code generated from contracts/fixtures/cards.json by internal/purchase/gen. DO NOT EDIT.")
	fmt.Fprintln(&buf, "package purchase")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "// defaultCards maps a simulator cardToken to its fixture PAN and expiry (see cardtokens.go's Ruling).")
	fmt.Fprintln(&buf, "var defaultCards = map[string]CardFixture{")
	for _, c := range fixture.Cards {
		fmt.Fprintf(&buf, "\t%q: {PAN: %q, ExpiryYYMM: %q},\n", c.CardToken, c.PAN, c.Expiry)
	}
	fmt.Fprintln(&buf, "}")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("cards_gen.go", formatted, 0o644); err != nil {
		log.Fatal(err)
	}
}
