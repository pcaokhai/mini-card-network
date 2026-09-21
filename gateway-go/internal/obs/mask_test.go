package obs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskPAN__MCN_005_AC2(t *testing.T) {
	cases := map[string]string{
		"card 9704360000004417 approved":                  "card 970436******4417 approved",
		"pan=4111111111111111111":                         "pan=411111*********1111",
		"rrn 626514000123 stan 000123":                    "rrn 626514000123 stan 000123",
		"de90 020000012409210732440000097049900000000000": "de90 020000012409210732440000097049900000000000",
		"amount 000000250000":                             "amount 000000250000",
	}
	for in, want := range cases {
		require.Equal(t, want, MaskPAN(in), in)
	}
}
