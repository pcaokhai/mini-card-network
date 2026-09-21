package isonet

import (
	"math/rand/v2"
	"time"
)

// Backoff computes full-jitter exponential backoff: delay = random(0, min(cap, base*2^attempt)).
type Backoff struct {
	Base time.Duration
	Cap  time.Duration
}

func (b Backoff) ceiling(attempt int) time.Duration {
	d := b.Base
	for i := 0; i < attempt; i++ {
		d *= 2
		if d >= b.Cap {
			return b.Cap
		}
	}
	return d
}

// Delay returns a jittered delay for the given attempt number (0-indexed).
func (b Backoff) Delay(attempt int) time.Duration {
	ceiling := b.ceiling(attempt)
	if ceiling <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(ceiling))) //nolint:gosec // jittered reconnect delay, not security-sensitive
}
