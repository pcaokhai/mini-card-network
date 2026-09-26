package api

import (
	"context"
	"net/http"
	"sync"
)

// replayStore gives a state-changing route docs/04 §2 idempotency: the same key and request get
// the stored response back, the same key with a different request gets 422.
// ponytail: in memory and never expired, so a restart forgets keys and the 24 h window is not
// enforced; move to a table (like purchase idempotency) if these routes need restart-safe replay.
type replayStore struct {
	mu    sync.Mutex
	byKey map[string]storedResponse
}

type storedResponse struct {
	fingerprint string
	status      int
	body        any
	location    string
}

func newReplayStore() *replayStore { return &replayStore{byKey: map[string]storedResponse{}} }

// serve replays key's stored response (status, body and Location header), or calls run and
// stores what it returns. run writes its own problem responses and returns ok=false for them, so
// failures are not replayed. The lock is
// held across run so two retries of one key can't both act.
func (s *replayStore) serve(req *http.Request, w http.ResponseWriter, key, fingerprint string, run func(ctx context.Context) (status int, body any, ok bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, found := s.byKey[key]; found {
		if prior.fingerprint != fingerprint {
			problem(w, req, http.StatusUnprocessableEntity, problemIdempotencyMismatch, "Idempotency-Key was already used with a different request")
			return
		}
		if prior.location != "" {
			w.Header().Set("Location", prior.location)
		}
		writeJSONBody(w, prior.status, prior.body)
		return
	}
	status, body, ok := run(req.Context())
	if !ok {
		return
	}
	s.byKey[key] = storedResponse{fingerprint: fingerprint, status: status, body: body, location: w.Header().Get("Location")}
	writeJSONBody(w, status, body)
}
