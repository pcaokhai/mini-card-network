// Package journey builds a plain-language transaction journey from stored tran_log state
// history, per docs/03-iso8583-interface-spec.md §8/§10.
package journey

// rcEasyText maps every response code (DE 39) from docs/03 §8 to a plain-language label.
var rcEasyText = map[string]string{
	"00": "Approved",
	"05": "Do not honor",
	"06": "Reversal error",
	"10": "Partially approved",
	"12": "Invalid transaction",
	"13": "Invalid amount",
	"14": "Unknown card",
	"17": "Cancelled by customer",
	"30": "Message format error",
	"51": "Insufficient funds",
	"54": "Card is expired",
	"55": "Incorrect PIN",
	"57": "Not allowed for this card",
	"61": "Limit exceeded",
	"62": "Card is blocked",
	"65": "Too many transactions",
	"68": "Response arrived too late",
	"75": "Too many PIN attempts",
	"91": "Link is down",
	"94": "Duplicate transaction",
	"95": "Reconciliation error",
	"96": "System error",
}

// EasyTextForRC returns the plain-language label for a response code, or a generic fallback for
// an RC not in docs/03 §8's table.
func EasyTextForRC(rc string) string {
	if text, ok := rcEasyText[rc]; ok {
		return text
	}
	return "Declined"
}
