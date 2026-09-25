package isonet

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/iso8583"
)

// signOnIssuer keeps sign-on state per acquirer, as the real issuer does: a sign-off from any
// session leaves every connection answering 91 until the acquirer signs on again.
type signOnIssuer struct {
	signedOn atomic.Bool
	signOns  atomic.Int32
}

func (f *signOnIssuer) serve(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.answer(conn)
		}
	}()
	return ln.Addr().String()
}

func (f *signOnIssuer) answer(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	for {
		payload, err := ReadFrame(conn)
		if err != nil {
			return
		}
		mti, fields, err := iso8583.Unpack(string(payload))
		if err != nil {
			return
		}
		fields[39] = f.respond(mti, fields[70])
		packed, err := iso8583.Pack(mti[:2]+"1"+mti[3:], fields)
		if err != nil || WriteFrame(conn, []byte(packed)) != nil {
			return
		}
	}
}

func (f *signOnIssuer) respond(mti, networkCode string) string {
	const networkManagement = "0800"
	switch {
	case mti == networkManagement && networkCode == "001":
		f.signedOn.Store(true)
		f.signOns.Add(1)
	case mti == networkManagement && networkCode == "002":
		f.signedOn.Store(false)
	case mti == networkManagement:
		// echo: answered whatever the sign-on state, which is why it can't detect this
	case !f.signedOn.Load():
		return "91"
	}
	return "00"
}

// Another session signing the acquirer off at the issuer (a test run against the dev stack did,
// 2026-09-25) left every purchase answered 91 while the gateway still showed SIGNED_ON.
func TestSupervisor_signsOnAgainWhenTheIssuerAnswersNotSignedOn__MCN_202(t *testing.T) {
	issuer := &signOnIssuer{}
	sup := NewSupervisor(Config{Addr: issuer.serve(t), EchoInterval: time.Minute, EchoFailureLimit: 3}, &fakeLinkStore{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = sup.Run(ctx) }()
	require.Eventually(t, func() bool { return issuer.signOns.Load() == 1 }, 2*time.Second, 10*time.Millisecond)

	issuer.signedOn.Store(false)
	resp, err := sup.Send(ctx, "0200", map[int]string{7: nowDE7(), 11: "000900", 3: "000000", 4: "000000001000", 41: "00000042", 49: "704"})
	require.NoError(t, err)
	require.Equal(t, "91", resp[39])

	require.Eventually(t, func() bool { return issuer.signOns.Load() == 2 }, 2*time.Second, 10*time.Millisecond, "re-signed on")
	resp, err = sup.Send(ctx, "0200", map[int]string{7: nowDE7(), 11: "000901", 3: "000000", 4: "000000001000", 41: "00000042", 49: "704"})
	require.NoError(t, err)
	require.Equal(t, "00", resp[39])
}
