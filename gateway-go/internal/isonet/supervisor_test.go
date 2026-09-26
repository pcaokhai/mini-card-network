package isonet

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/iso8583"
	"github.com/mcn/gateway-go/internal/store"
)

const signedOn = "SIGNED_ON"

// fakeLinkStore is written by the Supervisor's own goroutine and polled by require.Eventually
// from the test goroutine, so every access goes through mu.
type fakeLinkStore struct {
	mu       sync.Mutex
	statuses []string
	events   []string
	codes    []string
	echoes   int
	nextID   int64
}

func (f *fakeLinkStore) SetStatus(_ context.Context, _ string, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, status)
	return nil
}
func (f *fakeLinkStore) AppendEvent(_ context.Context, e store.NetworkEvent) (store.NetworkEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e.EasyText)
	f.codes = append(f.codes, e.Code)
	f.nextID++
	e.ID, e.OccurredAt = f.nextID, time.Now()
	return e, nil
}

func (f *fakeLinkStore) RecordEcho(context.Context, string, int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.echoes++
	return nil
}

func (f *fakeLinkStore) Get(_ context.Context, endpoint string) (store.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := ""
	if len(f.statuses) > 0 {
		status = f.statuses[len(f.statuses)-1]
	}
	return store.Link{Endpoint: endpoint, Status: status}, nil
}

func (f *fakeLinkStore) recorded() (codes []string, echoes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.codes...), f.echoes
}

func (f *fakeLinkStore) lastStatus() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.statuses) == 0 {
		return ""
	}
	return f.statuses[len(f.statuses)-1]
}

func (f *fakeLinkStore) hasStatus(status string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.statuses {
		if s == status {
			return true
		}
	}
	return false
}

// fakeIssuer answers every 0800 with 0810 RC 00, framed the same way the real issuer will be.
func fakeIssuer(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	return fakeIssuerSilentOn(t, "")
}

// fakeIssuerSilentOn is fakeIssuer, except it never answers a 0800 whose DE 70 is silentDE70.
func fakeIssuerSilentOn(t *testing.T, silentDE70 string) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				for {
					payload, err := ReadFrame(conn)
					if err != nil {
						return
					}
					_, fields, err := iso8583.Unpack(string(payload))
					if err != nil {
						return
					}
					if silentDE70 != "" && fields[70] == silentDE70 {
						continue
					}
					fields[39] = "00"
					packed, err := iso8583.Pack("0810", fields)
					if err != nil {
						return
					}
					if err := WriteFrame(conn, []byte(packed)); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

// fakeIssuerWithUnsolicitedLateResponse signs on normally, then writes one unsolicited 0210 that
// matches no pending request (simulating a response arriving after the caller's request already
// timed out and gave up).
func fakeIssuerWithUnsolicitedLateResponse(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		payload, err := ReadFrame(conn)
		if err != nil {
			return
		}
		_, fields, err := iso8583.Unpack(string(payload))
		if err != nil {
			return
		}
		fields[39] = "00"
		packed, err := iso8583.Pack("0810", fields)
		if err != nil {
			return
		}
		if err := WriteFrame(conn, []byte(packed)); err != nil {
			return
		}

		late := map[int]string{7: "0101120000", 11: "999999", 37: "626514000999", 39: "05"}
		latePacked, err := iso8583.Pack("0210", late)
		if err != nil {
			return
		}
		_ = WriteFrame(conn, []byte(latePacked))
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func TestSupervisor_callsLateResponseHandlerForUnmatchedResponse__MCN_403_AC1(t *testing.T) {
	addr, closeFn := fakeIssuerWithUnsolicitedLateResponse(t)
	defer closeFn()

	store := &fakeLinkStore{}
	sup := NewSupervisor(Config{Addr: addr, EchoInterval: time.Hour, EchoFailureLimit: 3}, store)

	var mu sync.Mutex
	var gotRRN, gotRC string
	sup.SetLateResponseHandler(func(_ string, fields map[int]string) {
		mu.Lock()
		defer mu.Unlock()
		gotRRN, gotRC = fields[37], fields[39]
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = sup.Run(ctx) }()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return gotRRN != ""
	}, time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "626514000999", gotRRN)
	require.Equal(t, "05", gotRC)
}

func TestSupervisor_connectsSignsOnAndEchoes__MCN_202_AC1(t *testing.T) {
	addr, closeFn := fakeIssuer(t)
	defer closeFn()

	store := &fakeLinkStore{}
	sup := NewSupervisor(Config{Addr: addr, EchoInterval: 50 * time.Millisecond, EchoFailureLimit: 3}, store)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- sup.Run(ctx) }()

	require.Eventually(t, func() bool {
		return store.lastStatus() == signedOn
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)
}

func TestSupervisor_triggerEchoSendsImmediately__MCN_204_AC3(t *testing.T) {
	addr, closeFn := fakeIssuer(t)
	defer closeFn()

	store := &fakeLinkStore{}
	sup := NewSupervisor(Config{Addr: addr, EchoInterval: time.Hour, EchoFailureLimit: 3}, store) // long interval: prove the trigger, not the ticker

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = sup.Run(ctx) }()

	require.Eventually(t, func() bool { return store.lastStatus() == signedOn }, time.Second, 10*time.Millisecond)

	result, err := sup.TriggerEcho(context.Background())
	require.NoError(t, err)
	require.True(t, result.OK)
	require.NotNil(t, result.LatencyMs)
	require.Equal(t, "00", *result.ResponseCode)
}

func TestSupervisor_triggerEchoReturnsNotOkWhenNotSignedOn__MCN_204_AC3(t *testing.T) {
	store := &fakeLinkStore{}
	sup := NewSupervisor(Config{Addr: "127.0.0.1:1", EchoInterval: time.Hour, EchoFailureLimit: 3}, store) // unreachable addr: never signs on

	result, err := sup.TriggerEcho(context.Background())
	require.NoError(t, err)
	require.False(t, result.OK)
}

func TestSupervisor_triggerSignOnFailsWhenNotConnected__MCN_204_AC3(t *testing.T) {
	store := &fakeLinkStore{}
	sup := NewSupervisor(Config{Addr: "127.0.0.1:1", EchoInterval: time.Hour, EchoFailureLimit: 3}, store)

	require.Error(t, sup.TriggerSignOn(context.Background()))
	require.Error(t, sup.TriggerSignOff(context.Background()))
}

type fakeHub struct {
	mu       sync.Mutex
	statuses []store.Link
	events   []store.NetworkEvent
}

func (h *fakeHub) BroadcastLinkStatus(l store.Link) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statuses = append(h.statuses, l)
}

func (h *fakeHub) BroadcastNetworkEvent(e store.NetworkEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e)
}

func (h *fakeHub) lastStatus() (store.Link, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.statuses) == 0 {
		return store.Link{}, false
	}
	return h.statuses[len(h.statuses)-1], true
}

func TestSupervisor_broadcastsLinkStatusOverHub__MCN_204_AC4(t *testing.T) {
	addr, closeFn := fakeIssuer(t)
	defer closeFn()

	store := &fakeLinkStore{}
	sup := NewSupervisor(Config{Addr: addr, EchoInterval: time.Hour, EchoFailureLimit: 3}, store)
	hub := &fakeHub{}
	sup.SetHub(hub)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = sup.Run(ctx) }()

	require.Eventually(t, func() bool {
		l, ok := hub.lastStatus()
		return ok && l.Status == signedOn
	}, time.Second, 10*time.Millisecond)
}

func TestSupervisor_marksDownAfterThreeFailedEchoes__MCN_202_AC2(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		payload, err := ReadFrame(conn) // sign-on: answer once
		if err != nil {
			return
		}
		_, fields, _ := iso8583.Unpack(string(payload))
		fields[39] = "00"
		packed, _ := iso8583.Pack("0810", fields)
		_ = WriteFrame(conn, []byte(packed))
		// then go silent: subsequent echoes time out
		time.Sleep(3 * time.Second)
	}()

	store := &fakeLinkStore{}
	sup := NewSupervisor(Config{Addr: ln.Addr().String(), EchoInterval: 20 * time.Millisecond, EchoTimeout: 15 * time.Millisecond, EchoFailureLimit: 3}, store)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = sup.Run(ctx) }()

	require.Eventually(t, func() bool {
		return store.hasStatus("DOWN")
	}, time.Second, 10*time.Millisecond)
}

// Like Toxiproxy before the issuer is up: the first connection is accepted and closed without an
// answer; later ones behave. The supervisor must not wait for the sign-on timeout (60 s here) on
// a connection that is already gone.
func TestSupervisor_reconnectsWhenTheConnectionDiesDuringSignOn__MCN_202(t *testing.T) {
	realAddr, closeReal := fakeIssuer(t)
	defer closeReal()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		first := true
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			if first {
				first = false
				_ = conn.Close()
				continue
			}
			upstream, err := net.Dial("tcp", realAddr)
			if err != nil {
				_ = conn.Close()
				continue
			}
			go func() { _, _ = io.Copy(upstream, conn) }()
			go func() { _, _ = io.Copy(conn, upstream) }()
		}
	}()

	store := &fakeLinkStore{}
	sup := NewSupervisor(Config{Addr: ln.Addr().String(), EchoInterval: time.Minute, EchoFailureLimit: 3,
		Backoff: Backoff{Base: 10 * time.Millisecond, Cap: 50 * time.Millisecond}}, store)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- sup.Run(ctx) }()

	require.Eventually(t, func() bool { return store.lastStatus() == signedOn }, 2*time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
}

func runSignedOn(t *testing.T, addr string, cfg Config) (*Supervisor, *fakeLinkStore, *fakeHub) {
	t.Helper()
	cfg.Addr = addr
	if cfg.EchoInterval == 0 {
		cfg.EchoInterval = time.Hour
	}
	cfg.EchoFailureLimit = 3
	links := &fakeLinkStore{}
	sup := NewSupervisor(cfg, links)
	hub := &fakeHub{}
	sup.SetHub(hub)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = sup.Run(ctx) }()
	require.Eventually(t, func() bool { return links.lastStatus() == signedOn }, time.Second, 10*time.Millisecond)
	return sup, links, hub
}

func (h *fakeHub) recorded() ([]store.Link, []store.NetworkEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]store.Link(nil), h.statuses...), append([]store.NetworkEvent(nil), h.events...)
}

func TestSupervisor_manualEchoRecordsAndBroadcasts__NET_G1_G2(t *testing.T) {
	addr, closeFn := fakeIssuer(t)
	defer closeFn()
	sup, links, hub := runSignedOn(t, addr, Config{})
	_, eventsBefore := hub.recorded()
	statusesBefore, _ := hub.recorded()

	result, err := sup.TriggerEcho(context.Background())
	require.NoError(t, err)
	require.True(t, result.OK)

	codes, echoes := links.recorded()
	require.Equal(t, 1, echoes, "a manual echo moves lastEchoAt")
	require.Equal(t, "ECHO_OK", codes[len(codes)-1])
	statuses, events := hub.recorded()
	require.Len(t, statuses, len(statusesBefore)+1)
	require.Len(t, events, len(eventsBefore)+1)
	require.Equal(t, "ECHO_OK", events[len(events)-1].Code)
	require.NotZero(t, events[len(events)-1].ID, "broadcast carries the stored row")
}

func TestSupervisor_manualSignOffAndSignOnWriteStatusAndEvents__NET_G2_G3(t *testing.T) {
	addr, closeFn := fakeIssuer(t)
	defer closeFn()
	sup, links, hub := runSignedOn(t, addr, Config{})

	require.NoError(t, sup.TriggerSignOff(context.Background()))
	require.Equal(t, "CONNECTED", links.lastStatus())
	last, _ := hub.lastStatus()
	require.Equal(t, "CONNECTED", last.Status)

	require.NoError(t, sup.TriggerSignOn(context.Background()))
	require.Equal(t, signedOn, links.lastStatus())

	codes, _ := links.recorded()
	require.Equal(t, []string{"SIGNED_OFF", "SIGNED_ON"}, codes[len(codes)-2:])
}

func TestSupervisor_signOffIsBoundedByTheEchoTimeout__NET_G9(t *testing.T) {
	addr, closeFn := fakeIssuerSilentOn(t, "002")
	defer closeFn()
	sup, links, _ := runSignedOn(t, addr, Config{EchoTimeout: 100 * time.Millisecond})

	start := time.Now()
	require.Error(t, sup.TriggerSignOff(context.Background()))
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, signedOn, links.lastStatus(), "a sign-off the issuer never confirmed changes nothing")
}

func TestNewSupervisor_capsTheDefaultEchoTimeoutBelowTheHTTPWriteTimeout__NET_G9(t *testing.T) {
	sup := NewSupervisor(Config{EchoInterval: time.Minute}, &fakeLinkStore{})
	require.Equal(t, 10*time.Second, sup.cfg.EchoTimeout)
}

func TestSupervisor_everyRecordedEventCarriesACode__NET_G15(t *testing.T) {
	addr, closeFn := fakeIssuer(t)
	defer closeFn()
	sup, links, _ := runSignedOn(t, addr, Config{})
	_, _ = sup.TriggerEcho(context.Background())
	_ = sup.TriggerSignOff(context.Background())
	_ = sup.TriggerSignOn(context.Background())

	codes, _ := links.recorded()
	require.NotEmpty(t, codes)
	for _, c := range codes {
		require.NotEmpty(t, c)
	}
}

func TestSupervisor_reportsLatencyAndInFlight__NET_G16(t *testing.T) {
	addr, closeFn := fakeIssuer(t)
	defer closeFn()
	sup, _, _ := runSignedOn(t, addr, Config{})

	_, err := sup.TriggerEcho(context.Background())
	require.NoError(t, err)

	p99, inFlight := sup.LinkMetrics()
	require.NotNil(t, p99)
	require.GreaterOrEqual(t, *p99, 0)
	require.Equal(t, 0, inFlight)
}
