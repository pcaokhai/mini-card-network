package journey

// Transaction types the journey tells apart (contracts/openapi.yaml TransactionType).
const (
	tranTypePreAuth    = "PREAUTH"
	tranTypeCompletion = "COMPLETION"
	tranTypeRefund     = "REFUND"
	tranTypeBalance    = "BALANCE"
)

// mtiPair is the request and response MTI of a transaction type (docs/03 §6).
func (b builder) mtiPair() (request, response string) {
	switch b.txn.Type {
	case tranTypePreAuth:
		return "0100", "0110"
	case tranTypeCompletion:
		return "0220", "0230"
	}
	return mtiRequest, "0210"
}

func (b builder) requestMTI() string {
	request, _ := b.mtiPair()
	return request
}

func (b builder) responseMTI() string {
	_, response := b.mtiPair()
	return response
}

// requestPath is the POS's call to the acquirer (contracts/openapi.yaml).
func (b builder) requestPath() string {
	switch b.txn.Type {
	case tranTypePreAuth:
		return "/v1/transactions/pre-authorizations"
	case tranTypeCompletion:
		return "/v1/transactions/" + orDash(b.txn.OriginalRRN) + "/completions"
	case tranTypeRefund:
		return "/v1/transactions/refunds"
	case tranTypeBalance:
		return "/v1/transactions/balance-inquiries"
	}
	return "/v1/transactions/purchases"
}

// carries reports whether the type's request (and so its response) holds de: a completion's
// 0220 carries no card-present data, and a balance inquiry requests no amount.
func (b builder) carries(de string) bool {
	switch b.txn.Type {
	case tranTypeCompletion:
		return de != "2" && de != "22" && de != "41" && de != "42"
	case tranTypeBalance:
		return de != "4" && de != "49"
	}
	return true
}

// approvedText is what an approval did for the cardholder.
func (b builder) approvedText() string {
	switch b.txn.Type {
	case tranTypePreAuth:
		return "The card's bank approved and held the money."
	case tranTypeRefund:
		return "The card's bank approved and gave the money back."
	case tranTypeBalance:
		return "The card's bank answered with the balance."
	}
	return "The card's bank approved and took the money."
}

// moneySign is +1 when the type credits the cardholder, -1 when it debits (or holds), 0 when it
// moves no money.
func moneySign(tranType string) int64 {
	switch tranType {
	case tranTypeRefund:
		return 1
	case tranTypeBalance:
		return 0
	}
	return -1
}
