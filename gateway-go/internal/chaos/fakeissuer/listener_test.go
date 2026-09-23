package fakeissuer

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/isonet"
)

func TestListener_acceptsAndNeverResponds__MCN_407_AC_DROP(t *testing.T) {
	l, err := NewListener("127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = l.Serve(ctx) }()

	conn, err := net.DialTimeout("tcp", l.Addr(), time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	require.NoError(t, isonet.WriteFrame(conn, []byte("fake-0200-payload")))

	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, err = isonet.ReadFrame(conn)
	require.Error(t, err) // deadline exceeded: nothing was ever written back
}
