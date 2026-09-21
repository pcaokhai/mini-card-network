package lab

import "testing"

func TestDecode_masksPANAndReturnsEasyAndTechnicalNames__MCN_103_AC1_AC4(t *testing.T) {
	d, err := Decode("0200723E448108E0920116970436000000441700000000000025000009210732080001231432080921281109215814051000697049962651400012300000042GOCPHO000000001CA PHE GOC PHO           HO CHI MINH  VN7047A3F09C21B84D6E00209F2608A1B2C3D4E5F607189F2701809F3602001C4E1D7B02C9A3F815")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if d.MTI != "0200" {
		t.Fatalf("MTI = %q", d.MTI)
	}
	var pan *Field
	for i := range d.Fields {
		if d.Fields[i].DE == "2" {
			pan = &d.Fields[i]
		}
	}
	if pan == nil {
		t.Fatal("DE 2 not found")
	}
	if pan.Value != "970436******4417" {
		t.Fatalf("PAN not masked: %q", pan.Value)
	}
	if pan.TechnicalName != "PAN" {
		t.Fatalf("technicalName = %q, want PAN", pan.TechnicalName)
	}
	for _, s := range d.Segments {
		if s.Key == "2" && s.Text != "16970436******4417" {
			t.Fatalf("PAN leaked via segment text: %q", s.Text)
		}
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
