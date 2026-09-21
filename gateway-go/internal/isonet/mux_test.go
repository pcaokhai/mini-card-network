package isonet

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/iso8583"
)

func TestMux_sendCorrelatesResponseByStanAndDE7__MCN_203_AC1(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()

	go func() {
		payload, err := ReadFrame(server)
		if err != nil {
			return
		}
		_, fields, err := iso8583.Unpack(string(payload))
		if err != nil {
			return
		}
		fields[39] = "00"
		packed, _ := iso8583.Pack("0810", fields)
		_ = WriteFrame(server, []byte(packed))
	}()

	mux := NewMux(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = mux.Serve(ctx) }()

	resp, err := mux.Send(context.Background(), "0800", map[int]string{7: "0101120000", 11: mux.NextSTAN(), 70: "301"})
	require.NoError(t, err)
	require.Equal(t, "00", resp[39])
}

func TestMux_sendTimesOutWithNoResponse__MCN_203_AC2(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()
	// server never responds

	mux := NewMux(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = mux.Serve(ctx) }()

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer sendCancel()
	_, err := mux.Send(sendCtx, "0800", map[int]string{7: "0101120000", 11: mux.NextSTAN(), 70: "301"})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMux_countsLateResponse__MCN_203_AC3(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()

	mux := NewMux(client)
	var lateCount int
	mux.OnLateResponse(func(string, map[int]string) { lateCount++ })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = mux.Serve(ctx) }()

	fields := map[int]string{7: "0101120000", 11: "000042", 39: "00"}
	packed, err := iso8583.Pack("0810", fields)
	require.NoError(t, err)
	require.NoError(t, WriteFrame(server, []byte(packed)))

	require.Eventually(t, func() bool { return lateCount == 1 }, time.Second, 10*time.Millisecond)
}
