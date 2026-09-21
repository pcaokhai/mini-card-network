// Package lab wraps internal/iso8583 with human-readable names and PAN masking for the
// Message Lab API (MCN-103).
package lab

import (
	"sort"
	"strconv"

	"github.com/mcn/gateway-go/internal/iso8583"
	"github.com/mcn/gateway-go/internal/obs"
)

// panDE is the data element carrying the PAN (contracts/iso8583/packager-spec.yaml).
const panDE = 2

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
		value := fields[n]
		if n == panDE {
			value = obs.MaskPAN(value)
		}
		out = append(out, Field{
			DE:            strconv.Itoa(n),
			EasyName:      easyName(n, spec.Name),
			TechnicalName: spec.Name,
			Format:        spec.Type,
			Value:         value,
			Raw:           segmentsByDE[n],
		})
	}
	return out
}

// Decode unpacks a raw wire message into the Lab API's DecodedMessage shape.
func Decode(raw string) (Decoded, error) {
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
	segByDE := make(map[int]string, len(segments))
	var primary, secondary string
	for _, s := range segments {
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
	maskedSegments := maskPANSegments(segments)
	d := Decoded{MTI: mti, PrimaryBitmap: primary, Segments: maskedSegments, Fields: buildFields(fields, segByDE)}
	if secondary != "" {
		d.SecondaryBitmap = &secondary
	}
	return d
}

// maskPANSegments masks DE 2's on-wire text before it leaves this package: buildFields already
// masks Field.Value, but Segments[] is a separate raw copy (MCN-103 AC4 requires PAN masked in
// every response field, including the highlighting data used by MCN-104).
func maskPANSegments(segments []Segment) []Segment {
	out := make([]Segment, len(segments))
	for i, s := range segments {
		if s.Key == strconv.Itoa(panDE) {
			s.Text = maskSegmentValue(panDE, s.Text)
		}
		out[i] = s
	}
	return out
}

// maskSegmentValue masks only the value portion of a segment's text, preserving any
// length-prefix digits (e.g. LL/LLL) so the on-wire framing stays visible for teaching.
func maskSegmentValue(de int, text string) string {
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
