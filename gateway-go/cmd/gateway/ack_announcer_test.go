package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/saf"
)

type ackedPort struct {
	saf.Port
	acked []int64
}

func (p *ackedPort) MarkAcked(_ context.Context, id int64) error {
	p.acked = append(p.acked, id)
	return nil
}

func TestReversalAckAnnouncer_announcesTheReversedTransaction__OVW_G3(t *testing.T) {
	port := &ackedPort{}
	var announced []string
	a := reversalAckAnnouncer{
		Port:     port,
		rrnOf:    func(context.Context, int64) (string, error) { return "626514000501", nil },
		announce: func(_ context.Context, rrn string) error { announced = append(announced, rrn); return nil },
		logger:   slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}

	require.NoError(t, a.MarkAcked(context.Background(), 7))

	require.Equal(t, []int64{7}, port.acked)
	require.Equal(t, []string{"626514000501"}, announced)
}
