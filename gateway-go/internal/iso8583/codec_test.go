package iso8583

import "testing"

func TestPack_derivesSecondaryBitmapAndRejectsExplicitDE1(t *testing.T) {
	_, err := Pack("0800", map[int]string{1: "0000000000000000"})
	var ce *CodecError
	if err == nil {
		t.Fatal("expected error when DE 1 is set explicitly")
	}
	if !AsCodecError(err, &ce) || ce.Code != "UNKNOWN_FIELD" {
		t.Fatalf("got %v, want UNKNOWN_FIELD", err)
	}
}

// The generated field table carries packager-spec's sensitivity flags, so consumers such as the
// Lab redact from the spec instead of a hand-kept list (LAB-G8).
func TestFields_carryThePackagerSpecSensitivity__LAB_G8(t *testing.T) {
	want := map[int]string{2: "pan", 14: "expiry", 48: "key-material", 52: "pin-block", 55: "emv", 64: "mac", 128: "mac"}
	for n, spec := range Fields {
		if spec.Sensitive != want[n] {
			t.Errorf("DE %d: sensitive %q, want %q", n, spec.Sensitive, want[n])
		}
	}
}
