package isonet

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// RRN is "Y DDD hh" + STAN (docs/03 §5), unique per day only while STANs don't repeat within an
// hour: a reconnect must not start the count again.
func TestSupervisor_stanCountsOnAcrossConnections__MCN_203(t *testing.T) {
	s := NewSupervisor(Config{}, nil)

	first := s.newMux(&bytes.Buffer{})
	require.Equal(t, "000001", first.NextSTAN())
	require.Equal(t, "000002", first.NextSTAN())

	reconnected := s.newMux(&bytes.Buffer{})
	require.Equal(t, "000003", reconnected.NextSTAN())
}

// After a restart the gateway resumes past the STANs already used under this hour's RRN prefix.
func TestSupervisor_resumesAfterTheLastUsedSTAN__MCN_203(t *testing.T) {
	s := NewSupervisor(Config{}, nil)

	s.ResumeSTANAfter(248)

	require.Equal(t, "000249", s.newMux(&bytes.Buffer{}).NextSTAN())
}

func TestSupervisor_neverMovesTheSTANBackwards__MCN_203(t *testing.T) {
	s := NewSupervisor(Config{}, nil)
	mux := s.newMux(&bytes.Buffer{})
	mux.NextSTAN()
	mux.NextSTAN()

	s.ResumeSTANAfter(1)

	require.Equal(t, "000003", mux.NextSTAN())
}

func TestSupervisor_runResumesFromTheConfiguredLastSTAN__MCN_203(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Run returns as soon as it has resumed
	s := NewSupervisor(Config{LastSTAN: func(context.Context) (int64, error) { return 41, nil }}, nil)

	require.NoError(t, s.Run(ctx))

	require.Equal(t, "000042", s.newMux(&bytes.Buffer{}).NextSTAN())
}
