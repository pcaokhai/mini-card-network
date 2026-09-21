package isonet

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/mcn/gateway-go/internal/obs"
)

// LinkStore is the persistence port the supervisor needs; internal/store implements it.
type LinkStore interface {
	SetStatus(ctx context.Context, endpoint, status string) error
	RecordEcho(ctx context.Context, endpoint string, latencyMs int) error
	RecordEvent(ctx context.Context, severity, easyText, technicalText string) error
}

// Config configures one Supervisor. Zero EchoTimeout means EchoInterval is used as the timeout.
type Config struct {
	Addr             string
	EchoInterval     time.Duration
	EchoTimeout      time.Duration
	EchoFailureLimit int
	Backoff          Backoff
}

const endpointName = "issuer" // v1 has exactly one link; the switch (Sprint 10) adds more.

// Supervisor keeps one link healthy: connect, sign on, echo, detect failure, reconnect.
type Supervisor struct {
	cfg   Config
	store LinkStore
	mux   *Mux
}

// NewSupervisor builds a Supervisor that reports link state through store.
func NewSupervisor(cfg Config, store LinkStore) *Supervisor {
	if cfg.EchoTimeout == 0 {
		cfg.EchoTimeout = cfg.EchoInterval
	}
	if cfg.Backoff == (Backoff{}) {
		cfg.Backoff = Backoff{Base: time.Second, Cap: 30 * time.Second}
	}
	return &Supervisor{cfg: cfg, store: store}
}

// Run supervises the link until ctx is cancelled.
func (s *Supervisor) Run(ctx context.Context) error {
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		_ = err // connection lifecycle errors are expected (reconnect loop); logging is the caller's job via obs
		obs.LinkUp.WithLabelValues(endpointName).Set(0)
		_ = s.store.SetStatus(ctx, endpointName, "DOWN")
		_ = s.store.RecordEvent(ctx, "WARN", "Link to issuer is down", "reconnecting with backoff")
		delay := s.cfg.Backoff.Delay(attempt)
		attempt++
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

// runOnce connects, signs on, and echoes until the link fails or ctx is cancelled.
func (s *Supervisor) runOnce(ctx context.Context) error {
	conn, err := net.Dial("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = s.store.SetStatus(ctx, endpointName, "CONNECTED")

	mux := NewMux(conn)
	mux.OnLateResponse(func(string, map[int]string) { obs.LateResponseTotal.Inc() })
	serveCtx, cancelServe := context.WithCancel(ctx)
	defer cancelServe()
	serveErr := make(chan error, 1)
	go func() { serveErr <- mux.Serve(serveCtx) }()

	s.mux = mux
	if err := s.signOn(ctx); err != nil {
		return err
	}
	obs.LinkUp.WithLabelValues(endpointName).Set(1)
	_ = s.store.SetStatus(ctx, endpointName, "SIGNED_ON")
	_ = s.store.RecordEvent(ctx, "INFO", "Link to issuer is up", "signed on")

	failures := 0
	ticker := time.NewTicker(s.cfg.EchoInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			signOffCtx, cancel := context.WithTimeout(context.Background(), s.cfg.EchoTimeout)
			_ = s.signOff(signOffCtx) //nolint:contextcheck // deliberately detached: ctx is already Done, sign-off still needs to send
			cancel()
			return nil
		case err := <-serveErr:
			return err
		case <-ticker.C:
			latency, err := s.echo(ctx)
			if err != nil {
				failures++
				if failures >= s.cfg.EchoFailureLimit {
					return err
				}
				continue
			}
			failures = 0
			_ = s.store.RecordEcho(ctx, endpointName, int(latency.Milliseconds()))
		}
	}
}

func (s *Supervisor) signOn(ctx context.Context) error {
	fields, err := s.mux.Send(ctx, "0800", map[int]string{7: nowDE7(), 11: s.mux.NextSTAN(), 70: "001"})
	if err != nil {
		return err
	}
	if fields[39] != "00" {
		return errRC(fields[39])
	}
	return nil
}

func (s *Supervisor) signOff(ctx context.Context) error {
	_, err := s.mux.Send(ctx, "0800", map[int]string{7: nowDE7(), 11: s.mux.NextSTAN(), 70: "002"})
	return err
}

func (s *Supervisor) echo(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	sendCtx, cancel := context.WithTimeout(ctx, s.cfg.EchoTimeout)
	defer cancel()
	fields, err := s.mux.Send(sendCtx, "0800", map[int]string{7: nowDE7(), 11: s.mux.NextSTAN(), 70: "301"})
	if err != nil {
		return 0, err
	}
	if fields[39] != "00" {
		return 0, errRC(fields[39])
	}
	return time.Since(start), nil
}

// nowDE7 formats the current time as DE 7 (MMDDhhmmss UTC, docs/03 §3).
func nowDE7() string {
	return time.Now().UTC().Format("0102150405")
}

func errRC(code string) error {
	return fmt.Errorf("unexpected response code %s", code)
}
