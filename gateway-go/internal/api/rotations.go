package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/rotation"
)

// Rotator is the port rotations.go needs; *rotation.Runner (via a thin adapter) satisfies it.
type Rotator interface {
	StartRotation(ctx context.Context, keyType string) (rotation.Row, error)
	GetRotation(ctx context.Context, id int64) (rotation.Row, error)
}

// keyRotationStep mirrors contracts/openapi.yaml's KeyRotation.steps[] entry.
type keyRotationStep struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	CompletedAt *time.Time `json:"completedAt"`
}

// keyRotation mirrors contracts/openapi.yaml's KeyRotation schema.
type keyRotation struct {
	RotationID string            `json:"rotationId"`
	KeyType    string            `json:"keyType"`
	Status     string            `json:"status"`
	NewKCV     *string           `json:"newKcv"`
	Steps      []keyRotationStep `json:"steps"`
}

// MountRotations registers the key rotation routes (contracts/openapi.yaml, tag "keys").
func MountRotations(r chi.Router, svc Rotator) {
	replays := newReplayStore()
	r.Post("/v1/keys/acquirer/rotations", handleStartRotation(svc, replays))
	r.Get("/v1/keys/acquirer/rotations/{rotationId}", handleGetRotation(svc))
}

// handleStartRotation starts a rotation and answers 202 with its RUNNING row at once; the steps
// run in the background and GET reports them, a failure included (SEC-G2/G3). A retry with the
// same Idempotency-Key gets the same rotation back (SEC-G4).
func handleStartRotation(svc Rotator, replays *replayStore) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		key, ok := requireUUIDKey(w, req)
		if !ok {
			return
		}
		var body struct {
			KeyType string `json:"keyType"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || (body.KeyType != "ZPK" && body.KeyType != "ZAK") {
			problem(w, http.StatusBadRequest, problemValidation, "keyType must be ZPK or ZAK")
			return
		}
		replays.serve(req.Context(), w, key, body.KeyType, func(ctx context.Context) (int, any, bool) {
			if body.KeyType == "ZAK" {
				// The issuer reads its ZAK from env once and never loads a rotated one, so after a
				// ZAK rotation plus a gateway restart every MAC would fail (RC 96).
				problem(w, http.StatusConflict, "conflict", "ZAK rotation disabled until the issuer loads its active ZAK from key_store; see SEC-G15")
				return 0, nil, false
			}
			row, err := svc.StartRotation(ctx, body.KeyType)
			if errors.Is(err, rotation.ErrRotationInProgress) {
				problem(w, http.StatusConflict, "conflict", "a key rotation is already running")
				return 0, nil, false
			}
			if err != nil {
				problem(w, http.StatusInternalServerError, problemInternal, "could not start the key rotation")
				return 0, nil, false
			}
			w.Header().Set("Location", "/v1/keys/acquirer/rotations/"+strconv.FormatInt(row.ID, 10))
			return http.StatusAccepted, toKeyRotation(row), true
		})
	}
}

func handleGetRotation(svc Rotator) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// Rotation ids are opaque strings on the wire: one that isn't ours is simply not found.
		id, err := strconv.ParseInt(chi.URLParam(req, "rotationId"), 10, 64)
		if err != nil {
			problem(w, http.StatusNotFound, problemNotFound, "no such key rotation")
			return
		}
		row, err := svc.GetRotation(req.Context(), id)
		if errors.Is(err, rotation.ErrNotFound) {
			problem(w, http.StatusNotFound, problemNotFound, "no such key rotation")
			return
		}
		if err != nil {
			problem(w, http.StatusInternalServerError, problemInternal, "could not read the key rotation")
			return
		}
		writeJSONBody(w, http.StatusOK, toKeyRotation(row))
	}
}

func toKeyRotation(row rotation.Row) keyRotation {
	steps := make([]keyRotationStep, 0, len(row.Steps))
	for _, s := range row.Steps {
		steps = append(steps, keyRotationStep{Name: s.Name, Status: s.Status, CompletedAt: s.CompletedAt})
	}
	return keyRotation{
		RotationID: strconv.FormatInt(row.ID, 10),
		KeyType:    row.KeyType,
		Status:     row.Status,
		NewKCV:     row.NewKCV,
		Steps:      steps,
	}
}
