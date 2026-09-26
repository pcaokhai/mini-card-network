package isonet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLatencyWindow_p99__NET_G16(t *testing.T) {
	var w latencyWindow
	require.Nil(t, w.p99Ms())

	for i := 1; i <= 100; i++ {
		w.add(time.Duration(i) * time.Millisecond)
	}
	require.Equal(t, 99, *w.p99Ms())

	for i := 0; i < latencyWindowSize; i++ {
		w.add(5 * time.Millisecond) // old samples roll out
	}
	require.Equal(t, 5, *w.p99Ms())
}
