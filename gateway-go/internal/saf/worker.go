// Package saf owns the store-and-forward queue's delivery worker and the reversal path that
// feeds it (docs/03-iso8583-interface-spec.md §7.3, §9).
package saf

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mcn/gateway-go/internal/hsm"
	"github.com/mcn/gateway-go/internal/iso8583"
	"github.com/mcn/gateway-go/internal/isonet"
	"github.com/mcn/gateway-go/internal/obs"
	"github.com/mcn/gateway-go/internal/store"
)

const (
	claimBatchSize = 50
	acknowledged   = "00"
)

// errNoLiveLink means there is no connection to allocate this advice's STAN on yet; the advice
// waits for the next attempt rather than going out without a MUX key.
var errNoLiveLink = errors.New("no live issuer link to allocate a STAN on")

// MuxSender sends on the acquirer's live issuer connection and allocates its STANs.
// *isonet.Supervisor satisfies it (the same shape purchase.Service uses).
type MuxSender interface {
	Send(ctx context.Context, mti string, fields map[int]string) (map[int]string, error)
	NextSTAN() (stan string, ok bool)
}

// CardPANs turns a simulator card token back into its PAN for DE 2, only while a frame is built.
// *purchase.CardTokenRegistry satisfies it.
type CardPANs interface {
	PAN(token string) (string, bool)
}

// Port is the saf_queue access Worker needs. *store.SafRepository satisfies it.
type Port interface {
	ClaimDue(ctx context.Context, limit int) ([]store.SafRow, error)
	MarkInFlight(ctx context.Context, id int64, attempts int, nextRetryAt time.Time, lastError string) error
	MarkAcked(ctx context.Context, id int64) error
	MarkDead(ctx context.Context, id int64, lastError string) error
	UpdatePayload(ctx context.Context, id int64, payload []byte) error
}

// Worker polls saf_queue and delivers due advices, repeating as x21 after the first attempt,
// full-jitter backing off between attempts, and dead-lettering after max_attempts.
type Worker struct {
	mux          MuxSender
	cards        CardPANs
	hsm          hsm.Module
	zak          []byte
	saf          Port
	encKey       []byte
	backoff      isonet.Backoff
	pollInterval time.Duration
}

// NewWorker builds a Worker. zak is the clear ZAK every advice is MACed under; encKey decrypts
// saf_queue.payload_enc (nil stores payloads unencrypted, which is safe only because they never
// hold a PAN).
func NewWorker(mux MuxSender, cards CardPANs, hsmModule hsm.Module, zak []byte, saf Port, encKey []byte, backoff isonet.Backoff, pollInterval time.Duration) *Worker {
	return &Worker{mux: mux, cards: cards, hsm: hsmModule, zak: zak, saf: saf, encKey: encKey, backoff: backoff, pollInterval: pollInterval}
}

// Run delivers due advices every pollInterval until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.deliverOnce(ctx); err != nil {
				return err
			}
		}
	}
}

// deliverOnce claims every due row and attempts one delivery each.
func (w *Worker) deliverOnce(ctx context.Context) error {
	due, err := w.saf.ClaimDue(ctx, claimBatchSize)
	if err != nil {
		return fmt.Errorf("claim due saf rows: %w", err)
	}
	for _, row := range due {
		if err := w.deliverRow(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

// deliverRow sends one advice. attempts==0 sends the original MTI; every repeat is x21 (docs/03
// §7.3: attempts > 0 means a repeat, never a second original send). Any response ACKs - advices
// are never declined by the issuer, only lost in transit.
func (w *Worker) deliverRow(ctx context.Context, row store.SafRow) error {
	mti := row.MTI
	if row.Attempts > 0 {
		mti = mti[:3] + "1"
	}

	adv, err := decodePayload(w.encKey, row.Payload)
	if err != nil {
		return fmt.Errorf("decode saf payload id=%d: %w", row.ID, err)
	}
	if err := w.assignNetworkIdentity(ctx, row.ID, &adv); err != nil {
		if errors.Is(err, errNoLiveLink) {
			return w.retryLater(ctx, row, err)
		}
		return err
	}
	frame, err := w.frame(mti, adv)
	if err != nil {
		return w.saf.MarkDead(ctx, row.ID, err.Error())
	}

	resp, err := w.mux.Send(ctx, mti, frame)
	if err != nil {
		return w.retryLater(ctx, row, err)
	}
	// Advices are never declined, so anything but "00" (91 link not signed on, 30 rejected frame)
	// means the issuer has not recorded the reversal yet (docs/03 §7.3).
	if rc := resp[39]; rc != acknowledged {
		return w.retryLater(ctx, row, fmt.Errorf("%s not acknowledged: DE 39 %q", mti[:3]+"0", rc))
	}
	return w.saf.MarkAcked(ctx, row.ID)
}

// assignNetworkIdentity gives an advice its STAN and DE 7 on its first attempt and persists them
// before anything is sent, so every x21 repeat is the same message with only the MTI changed
// (docs/03 §4) and matches the same MUX key (docs/03 §5).
func (w *Worker) assignNetworkIdentity(ctx context.Context, id int64, adv *advice) error {
	if adv.Fields[11] != "" {
		return nil
	}
	stan, ok := w.mux.NextSTAN()
	if !ok {
		return errNoLiveLink
	}
	adv.Fields[11] = stan
	adv.Fields[7] = time.Now().UTC().Format("0102150405")
	payload, err := encodePayload(w.encKey, *adv)
	if err != nil {
		return fmt.Errorf("encode saf payload id=%d: %w", id, err)
	}
	return w.saf.UpdatePayload(ctx, id, payload)
}

// frame is what goes on the wire for one attempt: the stored fields plus DE 2 from the card token
// and a MAC over the rest of the message, MTI included. DE 90 sets the secondary bitmap, so the
// MAC is DE 128 (docs/03 §11). An unresolvable token can never succeed, so it dead-letters.
func (w *Worker) frame(mti string, adv advice) (map[int]string, error) {
	frame := make(map[int]string, len(adv.Fields)+2)
	for n, v := range adv.Fields {
		frame[n] = v
	}
	pan, ok := w.cards.PAN(adv.CardToken)
	if !ok {
		return nil, fmt.Errorf("unknown card token %q: cannot build DE 2", adv.CardToken)
	}
	frame[2] = pan

	packed, err := iso8583.Pack(mti, frame)
	if err != nil {
		return nil, fmt.Errorf("pack for MAC: %w", err)
	}
	mac, err := w.hsm.ComputeMAC([]byte(packed), w.zak)
	if err != nil {
		return nil, fmt.Errorf("compute MAC: %w", err)
	}
	frame[128] = strings.ToUpper(hex.EncodeToString(mac))
	return frame, nil
}

// retryLater reschedules row with sendErr recorded as its last_error, or dead-letters it after
// max_attempts.
func (w *Worker) retryLater(ctx context.Context, row store.SafRow, sendErr error) error {
	attempts := row.Attempts + 1
	if attempts >= row.MaxAttempts {
		obs.SafDeadTotal.Inc()
		return w.saf.MarkDead(ctx, row.ID, sendErr.Error())
	}
	nextRetryAt := time.Now().Add(w.backoff.Delay(attempts))
	return w.saf.MarkInFlight(ctx, row.ID, attempts, nextRetryAt, sendErr.Error())
}
