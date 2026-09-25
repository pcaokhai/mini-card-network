package isonet

import (
	"slices"
	"sync"
	"time"
)

// latencyWindowSize is how many recent round trips p99 is computed over.
// ponytail: a fixed window sorted on read; a streaming quantile sketch if reads get hot.
const latencyWindowSize = 1000

// latencyWindow keeps the most recent round-trip latencies (echoes and requests) for the link's
// p99LatencyMs (NET-G16).
type latencyWindow struct {
	mu      sync.Mutex
	samples [latencyWindowSize]time.Duration
	next    int
	count   int
}

func (w *latencyWindow) add(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.samples[w.next] = d
	w.next = (w.next + 1) % latencyWindowSize
	w.count = min(w.count+1, latencyWindowSize)
}

// p99Ms is the nearest-rank 99th percentile in milliseconds, or nil before any sample.
func (w *latencyWindow) p99Ms() *int {
	w.mu.Lock()
	sorted := slices.Clone(w.samples[:w.count])
	w.mu.Unlock()
	if len(sorted) == 0 {
		return nil
	}
	slices.Sort(sorted)
	rank := (len(sorted)*99 + 99) / 100 // ceil(0.99 n)
	ms := int(sorted[rank-1].Milliseconds())
	return &ms
}
