package journey

import (
	"fmt"

	"github.com/mcn/gateway-go/internal/obs"
)

// IsoField mirrors contracts/openapi.yaml's IsoField schema.
type IsoField struct {
	DE            string `json:"de"`
	EasyName      string `json:"easyName"`
	TechnicalName string `json:"technicalName"`
	Format        string `json:"format"`
	Value         string `json:"value"`
}

// IsoMessage mirrors contracts/openapi.yaml's IsoMessage schema.
type IsoMessage struct {
	MTI    string     `json:"mti"`
	Fields []IsoField `json:"fields"`
}

// deMTI labels the MTI row, which the contract lists among the fields.
const deMTI = "MTI"

type fieldSpec struct{ easy, technical, format string }

// fieldSpecs holds names and formats from docs/03 §3. DE 52 and DE 64/128 are never shown: the
// PIN block is not stored and the MAC is computed per send (plan Rulings 1-2).
var fieldSpecs = map[string]fieldSpec{
	deMTI: {"Message type", "Message type indicator", "n 4"},
	"2":   {"Card number (masked)", "PAN", "n..19 LLVAR"},
	"3":   {"Transaction type", "Processing code", "n 6"},
	"4":   {"Amount", "Amount, transaction", "n 12"},
	"7":   {"Sent at (UTC)", "Transmission date/time", "n 10 MMDDhhmmss"},
	"11":  {"Trace number", "STAN", "n 6"},
	"22":  {"How the card was read", "POS entry mode", "n 3"},
	"37":  {"Reference number", "RRN", "an 12"},
	"38":  {"Approval code", "Authorization ID response", "an 6"},
	"39":  {"Result code", "Response code", "an 2"},
	"41":  {"Terminal ID", "Terminal ID", "ans 8"},
	"42":  {"Merchant ID", "Merchant ID", "ans 15"},
	"49":  {"Currency", "Currency code, transaction", "n 3"},
	"90":  {"Original payment", "Original data elements", "n 42"},
}

type de struct{ no, value string }

// newMessage keeps only the fields that are stored; values arrive in DE order.
func newMessage(mti string, values ...de) *IsoMessage {
	fields := []IsoField{isoField(deMTI, mti)}
	for _, v := range values {
		if v.value != "" {
			fields = append(fields, isoField(v.no, v.value))
		}
	}
	return &IsoMessage{MTI: mti, Fields: fields}
}

func isoField(no, value string) IsoField {
	spec := fieldSpecs[no]
	return IsoField{DE: no, EasyName: spec.easy, TechnicalName: spec.technical, Format: spec.format, Value: value}
}

// maskedPAN re-masks the stored masked PAN so a clear PAN can never reach a response, even one
// stored by mistake (root CLAUDE.md §6.2).
func (b builder) maskedPAN() de { return de{"2", obs.MaskPAN(b.txn.MaskedPAN)} }

func (b builder) request() *IsoMessage {
	t := b.txn
	return newMessage(mtiRequest, b.maskedPAN(), de{"3", t.ProcessingCode}, de{"4", amount12(t.Amount)},
		de{"7", b.requestDE7()}, de{"11", t.NetworkSTAN}, de{"22", t.POSEntryMode}, de{"37", t.RRN},
		de{"41", t.TerminalID}, de{"42", t.MerchantID}, de{"49", t.Currency})
}

// response echoes the request (docs/03 §3); DE 38 only on an approval (C2).
func (b builder) response() *IsoMessage {
	t := b.txn
	authCode := ""
	if t.ResponseCode == "00" || t.ResponseCode == "10" {
		authCode = t.AuthCode
	}
	return newMessage("0210", b.maskedPAN(), de{"3", t.ProcessingCode}, de{"4", amount12(t.Amount)},
		de{"7", b.requestDE7()}, de{"11", t.NetworkSTAN}, de{"37", t.RRN}, de{"38", authCode},
		de{"39", t.ResponseCode}, de{"41", t.TerminalID}, de{"42", t.MerchantID}, de{"49", t.Currency})
}

// advice is the stored 0420 as the SAF worker sends it, so DE 90 is exactly its layout.
func (b builder) advice(mti string) *IsoMessage {
	if b.rev == nil {
		return nil
	}
	f := b.rev.Fields
	return newMessage(mti, b.maskedPAN(), de{"3", f[3]}, de{"4", f[4]}, de{"7", f[7]}, de{"11", f[11]},
		de{"37", f[37]}, de{"39", f[39]}, de{"41", f[41]}, de{"42", f[42]}, de{"49", f[49]}, de{"90", f[90]})
}

// reversalAck is the 0430: the advice echoed with RC 00, the only RC that ACKs it (saf.Worker).
func (b builder) reversalAck() *IsoMessage {
	if b.rev == nil {
		return nil
	}
	f := b.rev.Fields
	return newMessage("0430", b.maskedPAN(), de{"3", f[3]}, de{"4", f[4]}, de{"7", f[7]}, de{"11", f[11]},
		de{"37", f[37]}, de{"39", "00"}, de{"41", f[41]}, de{"42", f[42]}, de{"49", f[49]})
}

func (b builder) requestDE7() string {
	if b.txn.SentAt == nil {
		return ""
	}
	return b.txn.SentAt.UTC().Format("0102150405")
}

func amount12(minor int64) string { return fmt.Sprintf("%012d", minor) }
