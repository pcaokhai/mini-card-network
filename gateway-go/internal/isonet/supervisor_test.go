package isonet

import (
	"context"
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
}

func (f *fakeLinkStore) SetStatus(_ context.Context, _ string, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, status)
	return nil
}
func (f *fakeLinkStore) RecordEvent(_ context.Context, _, easyText, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, easyText)
	return nil
}
func (f *fakeLinkStore) RecordEcho(context.Context, string, int) error { return nil }

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
