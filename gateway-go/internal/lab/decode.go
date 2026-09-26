// Package lab wraps internal/iso8583 with human-readable names and PAN masking for the
// Message Lab API (MCN-103).
package lab

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mcn/gateway-go/internal/iso8583"
)

// panDE is the data element carrying the PAN (contracts/iso8583/packager-spec.yaml).
const panDE = 2

// secretDEs never leave the Lab in any form, not even partially (engineering rule 2): PIN block,
// ICC data (EMV tags 5A and 57 carry the PAN and track 2 equivalent) and MACs.
// ponytail: duplicates packager-spec.yaml's sensitive flags (LAB-G8); generate from the spec once
// codegen carries them.
var secretDEs = map[int]bool{52: true, 55: true, 64: true, 128: true}

// freeTextPAN matches any run of 13+ digits in an/ans text, with no word boundaries: obs.MaskPAN's
// \b\d{13,19}\b misses a PAN glued to letters or "_" (CARD9704..., PAN_9704...) or inside a
// longer digit run. Over-masking a long reference number is the safe side in the Lab.
var freeTextPAN = regexp.MustCompile(`\d{13,}`)

// panVisibleHead / panVisibleTail are the PAN digits PCI DSS 3.4 allows on display.
const (
	panVisibleHead = 6
	panVisibleTail = 4
)

// Field is one decoded data element, ready for the Lab UI (contracts/openapi.yaml IsoField).
type Field struct {
	DE            string `json:"de"`
	EasyName      string `json:"easyName"`
	TechnicalName string `json:"technicalName"`
	Format        string `json:"format"`
	Value         string `json:"value"`
	Raw           string `json:"raw,omitempty"`
}

// Segment mirrors iso8583.Segment for JSON marshaling under this package's own name.
type Segment = iso8583.Segment

// Decoded is the Lab API's response shape (contracts/openapi.yaml DecodedMessage).
type Decoded struct {
	MTI             string    `json:"mti"`
	PrimaryBitmap   string    `json:"primaryBitmap"`
	SecondaryBitmap *string   `json:"secondaryBitmap,omitempty"`
	Segments        []Segment `json:"segments"`
	Fields          []Field   `json:"fields"`
	// Packed is the redacted wire message on encode (LAB-G3); null on decode.
	Packed *string `json:"packed"`
}

func buildFields(fields map[int]string, segmentsByDE map[int]string) []Field {
	numbers := make([]int, 0, len(fields))
	for n := range fields {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)
	out := make([]Field, 0, len(numbers))
	for _, n := range numbers {
		spec := iso8583.Fields[n]
		out = append(out, Field{
			DE:            strconv.Itoa(n),
			EasyName:      easyName(n, spec.Name),
			TechnicalName: spec.Name,
			Format:        spec.Type,
			Value:         redactValue(n, fields[n]),
			Raw:           segmentsByDE[n],
		})
	}
	return out
}

// Decode unpacks a raw wire message into the Lab API's DecodedMessage shape.
// A masked sample (see Samples) is swapped for its clear golden vector first, so the page can
// decode what the samples endpoint served without the PAN, PIN block or MAC ever leaving in clear.
func Decode(raw string) (Decoded, error) {
	if vector, ok := clearSamples[raw]; ok {
		raw = vector
	}
	mti, fields, segments, err := iso8583.UnpackSegmented(raw)
	if err != nil {
		return Decoded{}, err
	}
	return assemble(mti, fields, segments), nil
}

// Encode packs fields (keyed by DE number as a string, per the OpenAPI request body) and
// returns the same DecodedMessage shape as Decode, so the UI can show the breakdown either way.
func Encode(mti string, fields map[string]string) (Decoded, error) {
	numeric := make(map[int]string, len(fields))
	for k, v := range fields {
		n, convErr := strconv.Atoi(k)
		if convErr != nil {
			return Decoded{}, &iso8583.CodecError{Code: "UNKNOWN_FIELD", Detail: "DE key not numeric: " + k}
		}
		numeric[n] = v
	}
	packed, err := iso8583.Pack(mti, numeric)
	if err != nil {
		return Decoded{}, err
	}
	d, err := Decode(packed)
	if err != nil {
		return Decoded{}, err
	}
	redacted := joinSegments(d.Segments)
	d.Packed = &redacted
	return d, nil
}

func assemble(mti string, fields map[int]string, segments []Segment) Decoded {
	maskedSegments := redactSegments(segments)
	segByDE := make(map[int]string, len(segments))
	var primary, secondary string
	for _, s := range maskedSegments {
		switch s.Key {
		case "primaryBitmap":
			primary = s.Text
		case "secondaryBitmap":
			secondary = s.Text
		case "mti":
			// nothing to capture per-field
		default:
			n, _ := strconv.Atoi(s.Key)
			segByDE[n] = s.Text
		}
	}
	d := Decoded{MTI: mti, PrimaryBitmap: primary, Segments: maskedSegments, Fields: buildFields(fields, segByDE)}
	if secondary != "" {
		d.SecondaryBitmap = &secondary
	}
	return d
}

// redactSegments masks sensitive DEs' on-wire text before it leaves this package: Segments[] and
// Field.Raw are raw copies of the wire, separate from Field.Value (MCN-103 AC4, LAB-G1/G2).
func redactSegments(segments []Segment) []Segment {
	out := make([]Segment, len(segments))
	for i, s := range segments {
		if n, err := strconv.Atoi(s.Key); err == nil {
			s.Text = redactWire(n, s.Text)
		}
		out[i] = s
	}
	return out
}

// redactValue masks one field's value: DE 2 by position, secret DEs in full, and any PAN-like
// digit run in free text (an/ans) such as DE 48.
func redactValue(de int, value string) string {
	switch {
	case de == panDE:
		return maskPANByPosition(value)
	case secretDEs[de]:
		return strings.Repeat("*", len(value))
	case strings.HasPrefix(iso8583.Fields[de].Type, "an"):
		return freeTextPAN.ReplaceAllStringFunc(value, maskPANByPosition)
	}
	return value
}

// maskPANByPosition masks DE 2 whatever its length: obs.MaskPAN only matches 13-19 digit runs,
// but DE 2 is the PAN by definition. First 6 + last 4 when at least one digit stays hidden,
// otherwise only the last 4.
func maskPANByPosition(pan string) string {
	if len(pan) > panVisibleHead+panVisibleTail {
		return pan[:panVisibleHead] + strings.Repeat("*", len(pan)-panVisibleHead-panVisibleTail) + pan[len(pan)-panVisibleTail:]
	}
	if len(pan) > panVisibleTail {
		return strings.Repeat("*", len(pan)-panVisibleTail) + pan[len(pan)-panVisibleTail:]
	}
	return strings.Repeat("*", len(pan))
}

// redactWire is redactValue for on-wire text: the LL/LLL length prefix stays visible so the
// framing can still be taught, and redaction keeps the length.
func redactWire(de int, text string) string {
	prefixLen := 0
	switch iso8583.Fields[de].Prefix {
	case "LL":
		prefixLen = 2
	case "LLL":
		prefixLen = 3
	}
	if prefixLen > len(text) {
		return strings.Repeat("*", len(text))
	}
	return text[:prefixLen] + redactValue(de, text[prefixLen:])
}
