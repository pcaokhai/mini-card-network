package isonet

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/mcn/gateway-go/internal/iso8583"
)

type muxKey struct {
	stan string
	de7  string
}

// Mux correlates one connection's outbound requests to their inbound responses by (STAN, DE7)
// per docs/03 §5. It owns the connection's single STAN counter (docs/03-iso8583-interface-spec.md
// §5; MCN-203 ruling: STAN issuance moves here from Supervisor so sign-on/echo and financial
// traffic never collide on the same wire).
type Mux struct {
	conn    io.ReadWriter
	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[muxKey]chan map[int]string

	stan *atomic.Int64

	onLateResponse func(mti string, fields map[int]string)
}

// NewMux builds a Mux over conn with its own STAN count. Call Serve in its own goroutine to start
// reading responses.
func NewMux(conn io.ReadWriter) *Mux {
	return newMuxCounting(conn, new(atomic.Int64))
}

// newMuxCounting builds a Mux that draws STANs from a counter shared with other connections.
func newMuxCounting(conn io.ReadWriter, stan *atomic.Int64) *Mux {
	return &Mux{conn: conn, pending: make(map[muxKey]chan map[int]string), stan: stan}
}

// NextSTAN issues the next 6-digit STAN.
func (m *Mux) NextSTAN() string {
	n := m.stan.Add(1) % 1000000
	return fmt.Sprintf("%06d", n)
}

// Pending is the number of requests awaiting their response on this connection (NET-G16 inFlight).
func (m *Mux) Pending() int {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	return len(m.pending)
}

// OnLateResponse registers a callback invoked when a response arrives with no matching pending
// request — the caller already gave up (docs/03 §5: late-response detection, MCN-203-AC3).
func (m *Mux) OnLateResponse(fn func(mti string, fields map[int]string)) {
	m.onLateResponse = fn
}

// Send writes a request keyed on fields[11] (STAN) and fields[7] (DE7), then blocks for its
// correlated response until ctx is done (deadline or cancellation).
func (m *Mux) Send(ctx context.Context, mti string, fields map[int]string) (map[int]string, error) {
	key := muxKey{stan: fields[11], de7: fields[7]}
	ch := make(chan map[int]string, 1)

	m.pendingMu.Lock()
	m.pending[key] = ch
	m.pendingMu.Unlock()
	defer func() {
		m.pendingMu.Lock()
		delete(m.pending, key)
		m.pendingMu.Unlock()
	}()

	packed, err := iso8583.Pack(mti, fields)
	if err != nil {
		return nil, fmt.Errorf("pack request: %w", err)
	}

	// Write on its own goroutine: an unbuffered or slow-draining conn can block on Write
	// indefinitely (e.g. net.Pipe with no reader), and ctx must still be able to cancel Send.
	writeDone := make(chan error, 1)
	go func() {
		m.writeMu.Lock()
		defer m.writeMu.Unlock()
		writeDone <- WriteFrame(m.conn, []byte(packed))
	}()

	select {
	case err := <-writeDone:
		if err != nil {
			return nil, fmt.Errorf("write request: %w", err)
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Serve reads frames from the connection until ctx is cancelled or the connection errors,
// dispatching each to its matching pending Send call, or to onLateResponse when none matches.
func (m *Mux) Serve(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		payload, err := ReadFrame(m.conn)
		if err != nil {
			return err
		}
		mti, fields, err := iso8583.Unpack(string(payload))
		if err != nil {
			continue // malformed frame from the wire; drop and keep serving
		}
		key := muxKey{stan: fields[11], de7: fields[7]}

		m.pendingMu.Lock()
		ch, ok := m.pending[key]
		m.pendingMu.Unlock()

		if !ok {
			if m.onLateResponse != nil {
				m.onLateResponse(mti, fields)
			}
			continue
		}
		ch <- fields
	}
}
