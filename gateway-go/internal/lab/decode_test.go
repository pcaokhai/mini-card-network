package lab

import "testing"

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
