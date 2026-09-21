package isonet

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/iso8583"
)

type fakeLinkStore struct {
	statuses []string
	events   []string
}

func (f *fakeLinkStore) SetStatus(_ context.Context, _ string, status string) error {
	f.statuses = append(f.statuses, status)
	return nil
}
func (f *fakeLinkStore) RecordEvent(_ context.Context, _, easyText, _ string) error {
	f.events = append(f.events, easyText)
	return nil
}
func (f *fakeLinkStore) RecordEcho(context.Context, string, int) error { return nil }

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
				defer conn.Close()
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
		return len(store.statuses) > 0 && store.statuses[len(store.statuses)-1] == "SIGNED_ON"
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)
}

func TestSupervisor_marksDownAfterThreeFailedEchoes__MCN_202_AC2(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
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
		for _, s := range store.statuses {
			if s == "DOWN" {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
}
