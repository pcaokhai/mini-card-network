package isonet

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFrame_roundTrips__MCN_202_AC1(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteFrame(&buf, []byte("0800...")))
	got, err := ReadFrame(&buf)
	require.NoError(t, err)
	require.Equal(t, []byte("0800..."), got)
}

func TestWriteFrame_rejectsOversizedPayload__MCN_202_AC1(t *testing.T) {
	var buf bytes.Buffer
	require.Error(t, WriteFrame(&buf, make([]byte, maxFrameSize+1)))
}
