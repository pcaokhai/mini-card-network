package isonet

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/iso8583"
)

func TestMux_handles1000ConcurrentRequests__MCN_203_AC4(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	defer func() { _ = server.Close() }()

	// Echo server: read every frame, answer with the same fields plus RC 00.
	go func() {
		for {
			payload, err := ReadFrame(server)
			if err != nil {
				return
			}
			_, fields, err := iso8583.Unpack(string(payload))
			if err != nil {
				continue
			}
			fields[39] = "00"
			packed, err := iso8583.Pack("0810", fields)
			if err != nil {
				continue
			}
			if err := WriteFrame(server, []byte(packed)); err != nil {
				return
			}
		}
	}()

	mux := NewMux(client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = mux.Serve(ctx) }()

	const n = 1000
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			de7 := fmt.Sprintf("0101%06d", i)
			_, err := mux.Send(context.Background(), "0800", map[int]string{7: de7, 11: mux.NextSTAN(), 70: "301"})
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "request %d failed", i)
	}
}
