package isonet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackoff_growsAndCaps__MCN_202_AC2(t *testing.T) {
	b := Backoff{Base: time.Second, Cap: 30 * time.Second}
	for attempt := 0; attempt < 10; attempt++ {
		d := b.Delay(attempt)
		require.GreaterOrEqual(t, d, time.Duration(0))
		require.LessOrEqual(t, d, 30*time.Second)
	}
	require.LessOrEqual(t, b.ceiling(20), 30*time.Second)
}
