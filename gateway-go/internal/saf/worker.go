// Package saf owns the store-and-forward queue's delivery worker and the reversal path that
// feeds it (docs/03-iso8583-interface-spec.md §7.3, §9).
package saf

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
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
	// issuerNotSignedOn is the issuer's RC for a message on a link it has not signed on yet.
	issuerNotSignedOn = "91"
	// sendTimeout bounds the wait for one 0430, the same 30 s a purchase waits for its 0210
	// (docs/03 §9): the mux never fails a pending send when its connection drops.
	sendTimeout = 30 * time.Second
	// claimLease keeps a claimed row from being claimed again while it is being delivered, and
	// is how long a row waits after a crash or a failed store write before it is retried.
	claimLease = 2 * sendTimeout
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
	ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]store.SafRow, error)
	MarkInFlight(ctx context.Context, id int64, attempts int, nextRetryAt time.Time, lastError string) error
	MarkAcked(ctx context.Context, id int64) error
	MarkDead(ctx context.Context, id int64, lastError string) error
	UpdatePayload(ctx context.Context, id int64, payload []byte) error
}

// ResponseVerifier checks the MAC an issuer response carries, as the MTI it arrived as;
// purchase.MACVerifier satisfies it.
type ResponseVerifier interface {
	Verify(ctx context.Context, mti string, resp map[int]string) bool
}

// Worker polls saf_queue and delivers due advices, repeating as x21 after the first attempt,
// full-jitter backing off between attempts, and dead-lettering after max_attempts.
type Worker struct {
	mux          MuxSender
	cards        CardPANs
	hsm          hsm.Module
	zak          hsm.ZAKSource // read per send, so a rotated ZAK applies without a restart (SEC-G10)
	verifier     ResponseVerifier
	saf          Port
	encKey       []byte
	backoff      isonet.Backoff
	pollInterval time.Duration
	sendTimeout  time.Duration
	log          *slog.Logger
}

// NewWorker builds a Worker. zak is the clear ZAK every advice is MACed under; encKey decrypts
// saf_queue.payload_enc (nil stores payloads unencrypted, which is safe only because they never
// hold a PAN).
func NewWorker(mux MuxSender, cards CardPANs, hsmModule hsm.Module, zak hsm.ZAKSource, verifier ResponseVerifier, saf Port, encKey []byte, backoff isonet.Backoff, pollInterval time.Duration) *Worker {
	return &Worker{mux: mux, cards: cards, hsm: hsmModule, zak: zak, verifier: verifier, saf: saf, encKey: encKey, backoff: backoff, pollInterval: pollInterval, sendTimeout: sendTimeout, log: slog.Default()}
}

// SetLogger routes the worker's logs through the gateway's JSON logger.
func (w *Worker) SetLogger(l *slog.Logger) { w.log = l }

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

// deliverOnce claims every due row and attempts one delivery each. A failure is that row's alone:
// it is logged and the row comes back once its claim lease runs out, so a database hiccup never
// stops the worker or, through its errgroup, the gateway.
func (w *Worker) deliverOnce(ctx context.Context) error {
	due, err := w.saf.ClaimDue(ctx, claimBatchSize, claimLease)
	if err != nil {
		w.log.ErrorContext(ctx, "claim due saf rows", "err", err)
		return nil
	}
	for _, row := range due {
		if err := w.deliverRow(ctx, row); err != nil {
			w.log.ErrorContext(ctx, "deliver saf row", "saf_id", row.ID, "err", err)
		}
	}
	return nil
}

// deliverRow sends one advice. The first send is the original MTI; once a STAN has been assigned
// the advice may already be on the issuer's side, so every later send is an x21 repeat (docs/03
// §7.3), including one whose earlier ACK write failed. Only a "00" 0430 completes it.
func (w *Worker) deliverRow(ctx context.Context, row store.SafRow) error {
	adv, err := decodePayload(w.encKey, row.Payload)
	if err != nil {
		return fmt.Errorf("decode saf payload id=%d: %w", row.ID, err)
	}
	mti := row.MTI
	if adv.Fields[11] != "" {
		mti = mti[:3] + "1"
	}
	if err := w.assignNetworkIdentity(ctx, row.ID, &adv); err != nil {
		if errors.Is(err, errNoLiveLink) {
			return w.waitForLink(ctx, row, err)
		}
		return err
	}
	frame, err := w.frame(mti, adv)
	if err != nil {
		return w.saf.MarkDead(ctx, row.ID, err.Error())
	}

	sendCtx, cancel := context.WithTimeout(ctx, w.sendTimeout)
	defer cancel()
	resp, err := w.mux.Send(sendCtx, mti, frame)
	if errors.Is(err, isonet.ErrNotSignedOn) {
		return w.waitForLink(ctx, row, err)
	}
	if err != nil {
		return w.retryLater(ctx, row, err)
	}
	return w.settle(ctx, row, mti, resp)
}

// settle acknowledges row only for a response that proves the issuer recorded the advice.
func (w *Worker) settle(ctx context.Context, row store.SafRow, mti string, resp map[int]string) error {
	// Advices are never declined, so anything but "00" (91 link not signed on, 30 rejected frame)
	// means the issuer has not recorded the reversal yet (docs/03 §7.3). 91 is the link, not the
	// advice, so like a refused send it waits without counting an attempt.
	if resp[39] == issuerNotSignedOn {
		return w.waitForLink(ctx, row, fmt.Errorf("%s not acknowledged: DE 39 %q", mti[:3]+"0", issuerNotSignedOn))
	}
	if rc := resp[39]; rc != acknowledged {
		return w.retryLater(ctx, row, fmt.Errorf("%s not acknowledged: DE 39 %q", mti[:3]+"0", rc))
	}
	// Only the issuer holds the ZAK, so only a response whose MAC verifies proves the issuer
	// recorded the advice (NET-G20). A mismatch may be tampering or corruption: it is never an
	// acknowledgement, and the advice is repeated like any other unacknowledged send.
	if respMTI := responseMTI(mti); !w.verifier.Verify(ctx, respMTI, resp) {
		obs.MacFailureTotal.Inc()
		w.log.WarnContext(ctx, "advice response failed MAC verification; not acknowledged", "saf_id", row.ID, "mti", respMTI)
		return w.retryLater(ctx, row, fmt.Errorf("%s MAC verification failed", respMTI))
	}
	return w.saf.MarkAcked(ctx, row.ID)
}

// responseMTI is the issuer's answer to an advice sent as mti: its x30 (a 0420 or 0421 is answered
// by a 0430, a 0220 or 0221 by a 0230).
func responseMTI(mti string) string { return mti[:2] + "30" }

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
// and a MAC over the rest of the message, MTI included. Only the completion advice (0220/0221)
// goes without a PAN: it names the pre-auth by DE 37 and its terminal (docs/03 §3), while a STIP
// advice (0120) still carries its card.
// The MAC is DE 128 when a field above 64 (DE 90) sets the secondary bitmap, else DE 64 (docs/03
// §11). An unresolvable token can never succeed, so it dead-letters.
func (w *Worker) frame(mti string, adv advice) (map[int]string, error) {
	frame := make(map[int]string, len(adv.Fields)+2)
	macField := 64
	for n, v := range adv.Fields {
		frame[n] = v
		if n > 64 {
			macField = 128
		}
	}
	if !strings.HasPrefix(mti, "022") {
		pan, ok := w.cards.PAN(adv.CardToken)
		if !ok {
			return nil, fmt.Errorf("unknown card token %q: cannot build DE 2", adv.CardToken)
		}
		frame[2] = pan
	}

	packed, err := iso8583.Pack(mti, frame)
	if err != nil {
		return nil, fmt.Errorf("pack for MAC: %w", err)
	}
	mac, err := w.hsm.ComputeMAC([]byte(packed), w.zak.ActiveZAK())
	if err != nil {
		return nil, fmt.Errorf("compute MAC: %w", err)
	}
	frame[macField] = strings.ToUpper(hex.EncodeToString(mac))
	return frame, nil
}

// waitForLink reschedules row without counting an attempt: the link was down (nothing sent, or
// the issuer not signed on), so an outage can never dead-letter an advice.
func (w *Worker) waitForLink(ctx context.Context, row store.SafRow, why error) error {
	nextRetryAt := time.Now().Add(w.backoff.Delay(max(row.Attempts, 1)))
	return w.saf.MarkInFlight(ctx, row.ID, row.Attempts, nextRetryAt, why.Error())
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
