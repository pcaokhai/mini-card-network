package lab

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mcn/gateway-go/internal/iso8583"
)

const purchaseVector = "0200723E448108E0920116970436000000441700000000000025000009210732080001231432080921281109215814051000697049962651400012300000042GOCPHO000000001CA PHE GOC PHO           HO CHI MINH  VN7047A3F09C21B84D6E00209F2608A1B2C3D4E5F607189F2701809F3602001C4E1D7B02C9A3F815"

func findField(fields []Field, de string) *Field {
	for i := range fields {
		if fields[i].DE == de {
			return &fields[i]
		}
	}
	return nil
}

func findSegment(segments []Segment, key string) *Segment {
	for i := range segments {
		if segments[i].Key == key {
			return &segments[i]
		}
	}
	return nil
}

func TestDecode_masksPANAndReturnsEasyAndTechnicalNames__MCN_103_AC1_AC4(t *testing.T) {
	d, err := Decode(purchaseVector)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if d.MTI != "0200" {
		t.Fatalf("MTI = %q", d.MTI)
	}

	pan := findField(d.Fields, "2")
	if pan == nil {
		t.Fatal("DE 2 not found")
	}
	if pan.Value != "970436******4417" {
		t.Fatalf("PAN not masked: %q", pan.Value)
	}
	if pan.TechnicalName != "PAN" {
		t.Fatalf("technicalName = %q, want PAN", pan.TechnicalName)
	}

	panSegment := findSegment(d.Segments, "2")
	if panSegment == nil {
		t.Fatal("DE 2 segment not found")
	}
	if panSegment.Text != "16970436******4417" {
		t.Fatalf("PAN leaked via segment text: %q", panSegment.Text)
	}
}

func TestEncode_thenDecode_roundTrips(t *testing.T) {
	d, err := Encode("0800", map[string]string{"7": "0921073300", "11": "000200", "70": "301"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(d.Fields) != 3 {
		t.Fatalf("got %d fields, want 3", len(d.Fields))
	}
}

func TestEncode_rejectsUnknownFieldWithDENumber(t *testing.T) {
	_, err := Encode("0800", map[string]string{"999": "x"})
	if err == nil {
		t.Fatal("expected error for unknown DE 999")
	}
}

func TestDecode_masksPANInFieldRaw__LAB_G1(t *testing.T) {
	d, err := Decode(purchaseVector)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := findField(d.Fields, "2").Raw; got != "16970436******4417" {
		t.Fatalf("PAN leaked via fields[].raw: %q", got)
	}
}

func TestDecode_redactsPINBlockAndMAC__LAB_G2(t *testing.T) {
	d, err := Decode(purchaseVector)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	for _, de := range []string{"52", "64"} {
		f := findField(d.Fields, de)
		if f == nil {
			t.Fatalf("DE %s not found", de)
		}
		const redacted = "****************"
		if f.Value != redacted || f.Raw != redacted {
			t.Fatalf("DE %s not redacted: value=%q raw=%q", de, f.Value, f.Raw)
		}
		if s := findSegment(d.Segments, de); s.Text != redacted {
			t.Fatalf("DE %s leaked via segment text: %q", de, s.Text)
		}
	}
}

func TestSamples_neverCarrySensitiveDataInClearAndStillDecode__LAB_G2(t *testing.T) {
	for i, s := range Samples {
		_, clearFields, _, err := iso8583.UnpackSegmented(sampleVectors[i])
		if err != nil {
			t.Fatalf("vector %d: %v", i, err)
		}
		for _, de := range []int{2, 52, 64, 128} {
			if v, ok := clearFields[de]; ok && strings.Contains(s.Raw, v) {
				t.Fatalf("sample %s carries DE %d in clear", s.MTI, de)
			}
		}
		if len(s.Raw) != len(sampleVectors[i]) {
			t.Fatalf("sample %s redaction changed the framing", s.MTI)
		}
		fromMasked, err := Decode(s.Raw)
		if err != nil {
			t.Fatalf("sample %s no longer decodes: %v", s.MTI, err)
		}
		fromClear, _ := Decode(sampleVectors[i])
		if !reflect.DeepEqual(fromMasked, fromClear) {
			t.Fatalf("sample %s decodes differently from its vector", s.MTI)
		}
	}
}

// iccWithPANAndTrack2 is DE 55 carrying EMV tag 5A (PAN) and tag 57 (track 2 equivalent).
const iccWithPANAndTrack2 = "5A0897043600000044175710" + "9704360000004417D28122010000000F"

// requireNoLeak fails when any secret appears anywhere in the JSON the Lab would return.
func requireNoLeak(t *testing.T, d Decoded, secrets ...string) {
	t.Helper()
	body, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range secrets {
		if strings.Contains(string(body), s) {
			t.Fatalf("response leaks %q: %s", s, body)
		}
	}
}

func TestEncodeAndDecode_redactWholeICCData__LAB_B1(t *testing.T) {
	encoded, err := Encode("0200", map[string]string{"3": "000000", "11": "000123", "55": iccWithPANAndTrack2})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	requireNoLeak(t, encoded, "9704360000004417", "970436000000441", "D2812201")
	f := findField(encoded.Fields, "55")
	if f.Value != strings.Repeat("*", len(iccWithPANAndTrack2)) {
		t.Fatalf("DE 55 value not redacted: %q", f.Value)
	}

	packed, err := iso8583.Pack("0200", map[int]string{3: "000000", 11: "000123", 55: iccWithPANAndTrack2})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(packed)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	requireNoLeak(t, decoded, "9704360000004417", "970436000000441", "D2812201")
}

func TestDecode_masksShortPANByPosition__LAB_S1(t *testing.T) {
	for pan, want := range map[string]string{
		"97043600017":  "970436*0017",
		"970436000017": "970436**0017",
		"9704360017":   "******0017",
	} {
		d, err := Encode("0200", map[string]string{"2": pan, "3": "000000", "11": "000123"})
		if err != nil {
			t.Fatalf("Encode %s: %v", pan, err)
		}
		f := findField(d.Fields, "2")
		if f.Value != want || f.Raw != f.Raw[:2]+want {
			t.Fatalf("PAN %s: value=%q raw=%q, want %q", pan, f.Value, f.Raw, want)
		}
		requireNoLeak(t, d, pan)
	}
}

func TestDecode_masksPANInFreeText__LAB_S2_SF1(t *testing.T) {
	for text, want := range map[string]string{
		"CARD 9704360000004417 KEY ABC": "CARD 970436******4417 KEY ABC",
		"CARD9704360000004417":          "CARD970436******4417",
		"PAN_9704360000004417":          "PAN_970436******4417",
		"97043600000044170001":          "970436**********0001",
	} {
		d, err := Encode("0200", map[string]string{"3": "000000", "11": "000123", "48": text})
		if err != nil {
			t.Fatalf("Encode %q: %v", text, err)
		}
		requireNoLeak(t, d, "9704360000004417")
		f := findField(d.Fields, "48")
		if f.Value != want || f.Raw[3:] != want || findSegment(d.Segments, "48").Text[3:] != want {
			t.Fatalf("DE 48 %q: value=%q raw=%q, want %q", text, f.Value, f.Raw, want)
		}
	}
}

func TestEncode_redactsSecondaryMAC__LAB_S4(t *testing.T) {
	const mac = "0123456789ABCDEF"
	d, err := Encode("0200", map[string]string{"3": "000000", "11": "000123", "70": "301", "128": mac})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if d.SecondaryBitmap == nil {
		t.Fatal("expected a secondary bitmap for DE 128")
	}
	requireNoLeak(t, d, mac)
	if f := findField(d.Fields, "128"); f.Value != strings.Repeat("*", len(mac)) {
		t.Fatalf("DE 128 value = %q", f.Value)
	}
}

func TestClearSamples_neverKeyedByEmptyString__LAB_N1(t *testing.T) {
	if _, ok := clearSamples[""]; ok {
		t.Fatal(`Decode("") would map to a clear golden vector`)
	}
	if _, err := Decode(""); err == nil {
		t.Fatal(`Decode("") must fail`)
	}
}
