// Package fakeissuer is chaos-suite-only test infrastructure: a TCP listener that speaks the
// gateway's ISO 8583 framing (internal/isonet) just enough to accept a connection and read one
// request, then never writes a response - simulating the DROP_RESPONSE chaos scenario (MCN-407)
// so the gateway's own 30s request timeout (docs/03 §9) fires exactly as it would against a
// hung real issuer.
package fakeissuer

import (
	"context"
	"net"

	"github.com/mcn/gateway-go/internal/isonet"
)

// Listener accepts connections and never responds to them.
type Listener struct {
	ln net.Listener
}

// NewListener binds addr (use "127.0.0.1:0" for an OS-assigned port in tests).
func NewListener(addr string) (*Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Listener{ln: ln}, nil
}

// Addr returns the bound address.
func (l *Listener) Addr() string { return l.ln.Addr().String() }

// Close stops accepting new connections.
func (l *Listener) Close() error { return l.ln.Close() }

// Serve accepts connections until ctx is done. Each connection reads one framed message (the
// bytes are dropped - DROP_RESPONSE never decodes them) then parks until ctx is done, holding
// the connection open rather than resetting it.
func (l *Listener) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = l.ln.Close()
	}()
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go l.handle(ctx, conn)
	}
}

func (l *Listener) handle(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_, _ = isonet.ReadFrame(conn) // ponytail: dropped - the simulator never decodes ISO fields
	<-ctx.Done()
}
