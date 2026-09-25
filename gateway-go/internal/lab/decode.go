// Package lab wraps internal/iso8583 with human-readable names and PAN masking for the
// Message Lab API (MCN-103).
package lab

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mcn/gateway-go/internal/iso8583"
	"github.com/mcn/gateway-go/internal/obs"
)

// panDE is the data element carrying the PAN (contracts/iso8583/packager-spec.yaml).
const panDE = 2

// secretDEs never leave the Lab in any form, not even partially: PIN block and MACs (engineering rule 2).
var secretDEs = map[int]bool{52: true, 64: true, 128: true}

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
	return Decode(packed)
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

func redactValue(de int, value string) string {
	switch {
	case de == panDE:
		return obs.MaskPAN(value)
	case secretDEs[de]:
		return strings.Repeat("*", len(value))
	}
	return value
}

// redactWire is redactValue for on-wire text: PAN masking keeps the LL/LLL length prefix so the
// framing stays visible for teaching. Secret DEs are fixed-length, so they carry no prefix.
func redactWire(de int, text string) string {
	if de != panDE {
		return redactValue(de, text)
	}
	prefixLen := 0
	switch iso8583.Fields[de].Prefix {
	case "LL":
		prefixLen = 2
	case "LLL":
		prefixLen = 3
	}
	if prefixLen >= len(text) {
		return text
	}
	return text[:prefixLen] + obs.MaskPAN(text[prefixLen:])
}
