package iso8583

import (
	"path/filepath"
	"testing"

	moov "github.com/moov-io/iso8583"
	"github.com/moov-io/iso8583/encoding"
	"github.com/moov-io/iso8583/field"
	"github.com/moov-io/iso8583/padding"
	"github.com/moov-io/iso8583/prefix"
)

// buildMoovSpec mirrors Fields (generated from the same packager-spec.yaml) as a moov-io
// MessageSpec, so both libraries are driven by the identical field table — this is a
// cross-check of the *codec logic*, not of two independently-chosen specs.
//
// moov-io/iso8583 v0.26.1 quirks found empirically (its docs undersell how these fields
// actually need to be wired for MCN-87A's ASCII-hex-on-the-wire encoding):
//   - Field 1 must be a field.Bitmap (not a generic field); it manages primary/secondary
//     bitmaps itself, so our Fields[1] ("Secondary bitmap", 8 bytes) isn't used directly.
//   - Both Bitmap and Hex ("b" type) fields need Enc: encoding.BytesToASCIIHex, not
//     encoding.Binary as their package doc examples suggest — MCN-87A's wire format is
//     ASCII hex text (16 chars for an 8-byte bitmap, 2 chars/byte for "b" fields), not raw
//     binary bytes. BytesToASCIIHex's own doc warning ("don't use with String/Numeric/
//     Binary fields") refers to those Go field *types*, not field.Hex.
//   - Padding must only apply to *fixed*-length fields: applying padding.Left('0') to an
//     LL/LLL-prefixed numeric field (e.g. DE 2, PAN) makes moov encode it as a
//     fixed-width, zero-padded value instead of a variable-length one, producing a wrong
//     length prefix and extra characters on repack.
func buildMoovSpec() *moov.MessageSpec {
	spec := &moov.MessageSpec{
		Fields: map[int]field.Field{
			0: field.NewString(&field.Spec{Length: 4, Description: "MTI", Enc: encoding.ASCII, Pref: prefix.ASCII.Fixed}),
			1: field.NewBitmap(&field.Spec{Length: 8, Description: "Bitmap", Enc: encoding.BytesToASCIIHex, Pref: prefix.ASCII.Fixed}),
		},
	}
	for n, f := range Fields {
		if n == 1 {
			continue
		}
		pref := prefix.ASCII.Fixed
		switch f.Prefix {
		case "LL":
			pref = prefix.ASCII.LL
		case "LLL":
			pref = prefix.ASCII.LLL
		}
		if f.Type == "b" {
			spec.Fields[n] = field.NewHex(&field.Spec{Length: f.Length, Description: f.Name, Enc: encoding.BytesToASCIIHex, Pref: pref})
			continue
		}
		pad := padding.None
		if f.Prefix == "" {
			pad = padding.Left('0')
			if f.Type == "an" || f.Type == "ans" {
				pad = padding.Right(' ')
			}
		}
		spec.Fields[n] = field.NewString(&field.Spec{Length: f.Length, Description: f.Name, Enc: encoding.ASCII, Pref: pref, Pad: pad})
	}
	return spec
}

func TestCrossCheckWithMoovIO__MCN_101_AC4(t *testing.T) {
	spec := buildMoovSpec()
	paths, _ := filepath.Glob("../../../contracts/iso8583/vectors/*.json")
	if len(paths) == 0 {
		t.Fatal("no golden vectors found")
	}
	for _, p := range paths {
		p := p
		t.Run(filepath.Base(p), func(t *testing.T) {
			v := loadJSON[goldenVector](t, p)
			msg := moov.NewMessage(spec)
			if err := msg.Unpack([]byte(v.Packed)); err != nil {
				t.Fatalf("moov-io failed to unpack our golden vector: %v", err)
			}
			packedByMoov, err := msg.Pack()
			if err != nil {
				t.Fatalf("moov-io failed to repack: %v", err)
			}
			if string(packedByMoov) != v.Packed {
				t.Fatalf("moov-io repack differs from our packed bytes:\n moov: %s\n  our: %s", packedByMoov, v.Packed)
			}
		})
	}
}
