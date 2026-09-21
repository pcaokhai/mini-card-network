package isonet

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/mcn/gateway-go/internal/iso8583"
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
	stan  int
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

	if err := s.signOn(conn); err != nil {
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
			_ = s.signOff(conn)
			return nil
		case <-ticker.C:
			latency, err := s.echo(conn)
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

func (s *Supervisor) nextStan() string {
	s.stan++
	return padSTAN(s.stan)
}

func padSTAN(n int) string {
	digits := []byte("000000")
	for i := len(digits) - 1; n > 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits)
}

func (s *Supervisor) sendReceive(conn net.Conn, mti string, fields map[int]string) (map[int]string, error) {
	packed, err := iso8583.Pack(mti, fields)
	if err != nil {
		return nil, err
	}
	if err := WriteFrame(conn, []byte(packed)); err != nil {
		return nil, err
	}
	resp, err := ReadFrame(conn)
	if err != nil {
		return nil, err
	}
	_, respFields, err := iso8583.Unpack(string(resp))
	return respFields, err
}

func (s *Supervisor) signOn(conn net.Conn) error {
	fields, err := s.sendReceive(conn, "0800", map[int]string{7: nowDE7(), 11: s.nextStan(), 70: "001"})
	if err != nil {
		return err
	}
	if fields[39] != "00" {
		return errRC(fields[39])
	}
	return nil
}

func (s *Supervisor) signOff(conn net.Conn) error {
	_, err := s.sendReceive(conn, "0800", map[int]string{7: nowDE7(), 11: s.nextStan(), 70: "002"})
	return err
}

func (s *Supervisor) echo(conn net.Conn) (time.Duration, error) {
	start := time.Now()
	if err := conn.SetDeadline(start.Add(s.cfg.EchoTimeout)); err != nil {
		return 0, err
	}
	defer func() { _ = conn.SetDeadline(time.Time{}) }()
	fields, err := s.sendReceive(conn, "0800", map[int]string{7: nowDE7(), 11: s.nextStan(), 70: "301"})
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
