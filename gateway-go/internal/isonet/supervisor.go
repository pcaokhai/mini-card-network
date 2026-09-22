package isonet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/store"
)

// LinkStore is the persistence port the supervisor needs; internal/store implements it.
type LinkStore interface {
	SetStatus(ctx context.Context, endpoint, status string) error
	RecordEcho(ctx context.Context, endpoint string, latencyMs int) error
	RecordEvent(ctx context.Context, severity, easyText, technicalText string) error
}

// Hub is the broadcast port; internal/ws.Hub satisfies it.
type Hub interface {
	BroadcastLinkStatus(link store.Link)
	BroadcastNetworkEvent(evt store.NetworkEvent)
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

// ErrNotSignedOn is returned by Send, TriggerSignOn and TriggerSignOff when no connection is
// currently live.
var ErrNotSignedOn = errors.New("link is not signed on")

// EchoResult is the outcome of a manual echo trigger. A down link reports OK=false;
// it never surfaces as an error (MCN-204-AC3).
type EchoResult struct {
	OK           bool
	LatencyMs    *int
	ResponseCode *string
}

// Supervisor keeps one link healthy: connect, sign on, echo, detect failure, reconnect.
type Supervisor struct {
	cfg   Config
	store LinkStore
	hub   Hub

	// triggerMu guards mux so a manual trigger (TriggerEcho/TriggerSignOn/TriggerSignOff)
	// and the automatic echo ticker never send on the same Mux concurrently.
	triggerMu sync.Mutex
	mux       *Mux
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

// SetHub wires a Hub so every status/event transition is also broadcast over WebSocket.
func (s *Supervisor) SetHub(hub Hub) { s.hub = hub }

// TriggerEcho sends an out-of-band echo now, using the live connection's Mux.
// If the link is not currently signed on, it reports OK=false rather than an error.
func (s *Supervisor) TriggerEcho(ctx context.Context) (EchoResult, error) {
	s.triggerMu.Lock()
	defer s.triggerMu.Unlock()
	if s.mux == nil {
		return EchoResult{OK: false}, nil
	}
	start := time.Now()
	sendCtx, cancel := context.WithTimeout(ctx, s.cfg.EchoTimeout)
	defer cancel()
	fields, err := s.mux.Send(sendCtx, "0800", map[int]string{7: nowDE7(), 11: s.mux.NextSTAN(), 70: "301"})
	if err != nil {
		return EchoResult{OK: false}, nil
	}
	rc := fields[39]
	latencyMs := int(time.Since(start).Milliseconds())
	return EchoResult{OK: rc == "00", LatencyMs: &latencyMs, ResponseCode: &rc}, nil
}

// TriggerSignOn sends an out-of-band sign-on now, using the live connection's Mux.
func (s *Supervisor) TriggerSignOn(ctx context.Context) error {
	s.triggerMu.Lock()
	defer s.triggerMu.Unlock()
	if s.mux == nil {
		return ErrNotSignedOn
	}
	return s.signOn(ctx)
}

// TriggerSignOff sends an out-of-band sign-off now, using the live connection's Mux.
func (s *Supervisor) TriggerSignOff(ctx context.Context) error {
	s.triggerMu.Lock()
	defer s.triggerMu.Unlock()
	if s.mux == nil {
		return ErrNotSignedOn
	}
	return s.signOff(ctx)
}

// IsSignedOn reports whether a connection is currently live. Callers outside the supervision
// loop (e.g. purchase.Service) use this to short-circuit before attempting Send.
func (s *Supervisor) IsSignedOn() bool {
	s.triggerMu.Lock()
	defer s.triggerMu.Unlock()
	return s.mux != nil
}

// Send allocates the next STAN on the live connection and sends a request built from it, for
// callers outside the supervision loop (e.g. purchase.Service) that need to share the one
// connection's Mux rather than open a second one. STAN allocation and send happen under the same
// lock as every other trigger so they can never race a reconnect. Returns ErrNotSignedOn if no
// connection is currently live.
func (s *Supervisor) Send(ctx context.Context, mti string, buildFields func(stan string) map[int]string) (map[int]string, error) {
	s.triggerMu.Lock()
	defer s.triggerMu.Unlock()
	if s.mux == nil {
		return nil, ErrNotSignedOn
	}
	stan := s.mux.NextSTAN()
	return s.mux.Send(ctx, mti, buildFields(stan))
}

// setStatus updates the store and, if a Hub is wired, broadcasts the new link state.
func (s *Supervisor) setStatus(ctx context.Context, status string) {
	_ = s.store.SetStatus(ctx, endpointName, status)
	if s.hub != nil {
		s.hub.BroadcastLinkStatus(store.Link{Endpoint: endpointName, Status: status})
	}
}

// recordEvent inserts a network_event row and, if a Hub is wired, broadcasts it.
func (s *Supervisor) recordEvent(ctx context.Context, severity, easyText, technicalText string) {
	_ = s.store.RecordEvent(ctx, severity, easyText, technicalText)
	if s.hub != nil {
		s.hub.BroadcastNetworkEvent(store.NetworkEvent{Severity: severity, EasyText: easyText, TechnicalText: technicalText})
	}
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
		s.setStatus(ctx, "DOWN")
		s.recordEvent(ctx, "WARN", "Link to issuer is down", "reconnecting with backoff")
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
	s.setStatus(ctx, "CONNECTED")

	mux := NewMux(conn)
	mux.OnLateResponse(func(string, map[int]string) { obs.LateResponseTotal.Inc() })
	serveCtx, cancelServe := context.WithCancel(ctx)
	defer cancelServe()
	serveErr := make(chan error, 1)
	go func() { serveErr <- mux.Serve(serveCtx) }()

	s.triggerMu.Lock()
	s.mux = mux
	s.triggerMu.Unlock()
	defer func() {
		s.triggerMu.Lock()
		s.mux = nil
		s.triggerMu.Unlock()
	}()

	if err := s.signOn(ctx); err != nil {
		return err
	}
	obs.LinkUp.WithLabelValues(endpointName).Set(1)
	s.setStatus(ctx, "SIGNED_ON")
	s.recordEvent(ctx, "INFO", "Link to issuer is up", "signed on")

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
			s.triggerMu.Lock()
			latency, err := s.echo(ctx)
			s.triggerMu.Unlock()
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
