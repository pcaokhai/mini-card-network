package iso8583

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type goldenVector struct {
	Name   string            `json:"name"`
	MTI    string            `json:"mti"`
	Fields map[string]string `json:"fields"`
	Packed string            `json:"packed"`
}

type invalidVector struct {
	Name          string `json:"name"`
	Packed        string `json:"packed"`
	ExpectedError string `json:"expectedError"`
}

func loadJSON[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test fixture path built from a controlled glob, not user input
	if err != nil {
		t.Fatal(err)
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// keyedByDE converts the vector JSON's string-keyed DE map into the int-keyed map Pack/Unpack use.
func keyedByDE(m map[string]string) map[int]string {
	out := make(map[int]string, len(m))
	for k, v := range m {
		n := 0
		for _, c := range k {
			n = n*10 + int(c-'0')
		}
		out[n] = v
	}
	return out
}

func assertVectorRoundTrips(t *testing.T, v goldenVector) {
	t.Helper()
	fields := keyedByDE(v.Fields)
	packed, err := Pack(v.MTI, fields)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if packed != v.Packed {
		t.Fatalf("pack mismatch\n got: %s\nwant: %s", packed, v.Packed)
	}
	mti, back, err := Unpack(v.Packed)
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if mti != v.MTI {
		t.Fatalf("mti mismatch: got %s want %s", mti, v.MTI)
	}
	for de, want := range keyedByDE(v.Fields) {
		if back[de] != want {
			t.Fatalf("DE %d mismatch: got %s want %s", de, back[de], want)
		}
	}
}

func TestGoldenVectors_roundTripByteIdentical__MCN_101_AC1(t *testing.T) {
	paths, _ := filepath.Glob("../../../contracts/iso8583/vectors/*.json")
	if len(paths) == 0 {
		t.Fatal("no golden vectors found")
	}
	for _, p := range paths {
		p := p
		t.Run(filepath.Base(p), func(t *testing.T) {
			assertVectorRoundTrips(t, loadJSON[goldenVector](t, p))
		})
	}
}

func TestInvalidVectors_returnExpectedErrorCode__MCN_101_AC2(t *testing.T) {
	paths, _ := filepath.Glob("../../../contracts/iso8583/vectors/invalid/*.json")
	if len(paths) == 0 {
		t.Fatal("no invalid vectors found")
	}
	for _, p := range paths {
		p := p
		t.Run(filepath.Base(p), func(t *testing.T) {
			v := loadJSON[invalidVector](t, p)
			_, _, err := Unpack(v.Packed)
			if err == nil {
				t.Fatalf("expected error %s, got success", v.ExpectedError)
			}
			var ce *CodecError
			if !AsCodecError(err, &ce) {
				t.Fatalf("expected *CodecError, got %T: %v", err, err)
			}
			if ce.Code != v.ExpectedError {
				t.Fatalf("expected %s, got %s (%v)", v.ExpectedError, ce.Code, err)
			}
		})
	}
}
