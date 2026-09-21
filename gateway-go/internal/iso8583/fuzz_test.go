package iso8583

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func FuzzUnpack(f *testing.F) {
	for _, pattern := range []string{"../../../contracts/iso8583/vectors/*.json", "../../../contracts/iso8583/vectors/invalid/*.json"} {
		paths, _ := filepath.Glob(pattern)
		for _, p := range paths {
			data, err := os.ReadFile(p) //nolint:gosec // test fixture path built from a controlled glob, not user input
			if err != nil {
				continue
			}
			var v struct {
				Packed string `json:"packed"`
			}
			if json.Unmarshal(data, &v) == nil && v.Packed != "" {
				f.Add(v.Packed)
			}
		}
	}
	f.Fuzz(func(_ *testing.T, packed string) {
		// Unpack must never panic on arbitrary input; any error is acceptable.
		_, _, _ = Unpack(packed)
	})
}
