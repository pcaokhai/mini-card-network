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
