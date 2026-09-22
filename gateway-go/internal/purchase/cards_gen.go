// Code generated from contracts/fixtures/cards.json by internal/purchase/gen. DO NOT EDIT.
package purchase

// defaultCards maps a simulator cardToken to its fixture PAN and expiry (see cardtokens.go's Ruling).
var defaultCards = map[string]CardFixture{
	"tok_normal":  {PAN: "9704360000004417", ExpiryYYMM: "2811"},
	"tok_low":     {PAN: "9704360000009021", ExpiryYYMM: "2903"},
	"tok_blocked": {PAN: "9704360000003310", ExpiryYYMM: "2707"},
	"tok_expired": {PAN: "9704360000007765", ExpiryYYMM: "2608"},
}
