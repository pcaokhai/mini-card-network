// Code generated from contracts/fixtures/cards.json by internal/purchase/gen. DO NOT EDIT.
package purchase

// defaultCards maps a simulator cardToken to its fixture PAN, expiry, balance and currency (see cardtokens.go's Ruling).
var defaultCards = map[string]CardFixture{
	"tok_normal":  {PAN: "9704360000004417", ExpiryYYMM: "2811", Balance: 5000000, Currency: "704"},
	"tok_low":     {PAN: "9704360000009021", ExpiryYYMM: "2903", Balance: 80000, Currency: "704"},
	"tok_blocked": {PAN: "9704360000003310", ExpiryYYMM: "2707", Balance: 2000000, Currency: "704"},
	"tok_expired": {PAN: "9704360000007765", ExpiryYYMM: "2608", Balance: 1500000, Currency: "704"},
	"tok_limit":   {PAN: "9704360000001208", ExpiryYYMM: "2912", Balance: 50000000, Currency: "704"},
	"tok_second":  {PAN: "9704360000005540", ExpiryYYMM: "3006", Balance: 200000000, Currency: "704"},
}
