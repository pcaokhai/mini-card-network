package iso8583

import "testing"

func TestUnpackSegmented_capturesMTIBitmapAndEachField__MCN_103_AC1(t *testing.T) {
	// 0800 echo vector: DE 7, DE 11, DE 70.
	_, _, segments, err := UnpackSegmented("0800822000000000000004000000000000000921073300000200301")
	if err != nil {
		t.Fatalf("UnpackSegmented: %v", err)
	}
	keys := make([]string, len(segments))
	for i, s := range segments {
		keys[i] = s.Key
	}
	// DE 70 requires the secondary bitmap (bit 70 > 64), so it must appear as its own segment.
	want := []string{"mti", "primaryBitmap", "secondaryBitmap", "7", "11", "70"}
	if len(keys) != len(want) {
		t.Fatalf("got keys %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("segment %d: got %s want %s", i, keys[i], want[i])
		}
	}
	if segments[0].Text != "0800" {
		t.Fatalf("mti segment text = %q, want 0800", segments[0].Text)
	}
}
