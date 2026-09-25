package lab

import (
	"strings"

	"github.com/mcn/gateway-go/internal/iso8583"
)

// Sample is one entry for GET /v1/lab/messages/samples.
type Sample struct {
	MTI   string `json:"mti"`
	Label string `json:"label"`
	Raw   string `json:"raw"`
}

// sampleVectors are the four golden vectors (contracts/iso8583/vectors), in Samples order.
// Hardcoded rather than read from disk at runtime: the binary should work without contracts/
// present in its container image, and these four are stable, versioned wire formats.
// They never leave the package in clear: Samples serves them redacted and Decode maps back.
var sampleVectors = []string{
	"0200723E448108E0920116970436000000441700000000000025000009210732080001231432080921281109215814051000697049962651400012300000042GOCPHO000000001CA PHE GOC PHO           HO CHI MINH  VN7047A3F09C21B84D6E00209F2608A1B2C3D4E5F607189F2701809F3602001C4E1D7B02C9A3F815",
	"0210723A00010EC0800116970436000000441700000000000025000009210732080001231432080921092106970499626514000123A001230000000042GOCPHO00000000170491C0A4E27D3B5F68",
	"0420F23A04810AC08000000000400000000116970436000000441700000000000060000009210733140001251432440921092105100069704996265140001246800000042GOCPHO0000000017040200000124092107324400000970499000000000005D0E33A17BC2904F",
	"0800822000000000000004000000000000000921073300000200301",
}

// Samples returns the golden vectors as human-labeled samples, with the PAN masked and the
// PIN block and MAC redacted in raw (LAB-G2).
var Samples = []Sample{
	{MTI: "0200", Label: "Purchase, chip + PIN", Raw: mustRedactRaw(sampleVectors[0])},
	{MTI: "0210", Label: "Purchase approved", Raw: mustRedactRaw(sampleVectors[1])},
	{MTI: "0420", Label: "Reversal on timeout", Raw: mustRedactRaw(sampleVectors[2])},
	{MTI: "0800", Label: "Echo (network management)", Raw: mustRedactRaw(sampleVectors[3])},
}

// clearSamples maps each served (redacted) sample back to its golden vector for Decode.
var clearSamples = func() map[string]string {
	m := make(map[string]string, len(sampleVectors))
	for _, v := range sampleVectors {
		m[mustRedactRaw(v)] = v
	}
	return m
}()

// mustRedactRaw rebuilds a golden vector from its redacted segments (redaction keeps every
// segment's length, so the framing is unchanged). It runs only at package init on the fixed
// vectors above: a vector that stops parsing is a build defect, and falling back to "" would
// let Decode("") map to a clear vector.
func mustRedactRaw(raw string) string {
	_, _, segments, err := iso8583.UnpackSegmented(raw)
	if err != nil {
		panic("lab: golden sample vector no longer parses: " + err.Error())
	}
	return joinSegments(redactSegments(segments))
}

func joinSegments(segments []Segment) string {
	var b strings.Builder
	for _, s := range segments {
		b.WriteString(s.Text)
	}
	return b.String()
}
