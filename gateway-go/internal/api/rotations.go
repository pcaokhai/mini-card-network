package api

import (
	"context"
	"encoding/json"
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
	r.Post("/v1/keys/acquirer/rotations", handleStartRotation(svc))
	r.Get("/v1/keys/acquirer/rotations/{rotationId}", handleGetRotation(svc))
}

func handleStartRotation(svc Rotator) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Idempotency-Key") == "" {
			problem(w, http.StatusBadRequest, "idempotency-key-required", "Idempotency-Key header is required")
			return
		}
		var body struct {
			KeyType string `json:"keyType"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			problem(w, http.StatusBadRequest, "invalid-request", err.Error())
			return
		}
		row, err := svc.StartRotation(req.Context(), body.KeyType)
		if err != nil {
			problem(w, http.StatusInternalServerError, "rotation-failed", err.Error())
			return
		}
		writeJSONBody(w, http.StatusAccepted, toKeyRotation(row))
	}
}

func handleGetRotation(svc Rotator) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(req, "rotationId"), 10, 64)
		if err != nil {
			problem(w, http.StatusBadRequest, "invalid-rotation-id", err.Error())
			return
		}
		row, err := svc.GetRotation(req.Context(), id)
		if err != nil {
			problem(w, http.StatusInternalServerError, "get-rotation-failed", err.Error())
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
