package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/rotation"
)

type fakeRotator struct{ result rotation.Row }

func (f *fakeRotator) StartRotation(_ context.Context, _ string) (rotation.Row, error) {
	return f.result, nil
}
func (f *fakeRotator) GetRotation(_ context.Context, _ int64) (rotation.Row, error) {
	return f.result, nil
}

func TestPostKeysAcquirerRotations_returns202WithKeyRotation__MCN_504_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{result: rotation.Row{ID: 1, KeyType: "ZPK", Status: "COMPLETED"}})

	body, _ := json.Marshal(map[string]string{"keyType": "ZPK"})
	req := httptest.NewRequest(http.MethodPost, "/v1/keys/acquirer/rotations", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "11111111-1111-1111-1111-111111111111")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Contains(t, rec.Body.String(), `"keyType":"ZPK"`)
}

func TestPostKeysAcquirerRotations_requiresIdempotencyKey__MCN_504_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{})

	body, _ := json.Marshal(map[string]string{"keyType": "ZPK"})
	req := httptest.NewRequest(http.MethodPost, "/v1/keys/acquirer/rotations", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetKeysAcquirerRotation_returns200__MCN_504_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{result: rotation.Row{ID: 7, KeyType: "ZAK", Status: "RUNNING"}})

	req := httptest.NewRequest(http.MethodGet, "/v1/keys/acquirer/rotations/7", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"RUNNING"`)
}
