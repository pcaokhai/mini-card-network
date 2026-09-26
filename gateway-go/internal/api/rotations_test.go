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

type fakeRotator struct {
	result   rotation.Row
	startErr error
	getErr   error
	starts   int
}

func (f *fakeRotator) StartRotation(_ context.Context, _ string) (rotation.Row, error) {
	f.starts++
	return f.result, f.startErr
}
func (f *fakeRotator) GetRotation(_ context.Context, _ int64) (rotation.Row, error) {
	return f.result, f.getErr
}

const rotationsPath = "/v1/keys/acquirer/rotations"

func postRotation(t *testing.T, r http.Handler, key, keyType string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(struct {
		KeyType string `json:"keyType"`
	}{keyType})
	req := httptest.NewRequest(http.MethodPost, rotationsPath, bytes.NewReader(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	return rec.Code, got
}

func TestPostRotation_returnsRunningAtOnce__SEC_G2(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{result: rotation.Row{ID: 3, KeyType: keyZPK, Status: rotationRunning}})

	code, got := postRotation(t, r, testLinkKey, keyZPK)
	require.Equal(t, http.StatusAccepted, code)
	require.Equal(t, rotationRunning, got["status"])
	require.Equal(t, "3", got["rotationId"])
}

func TestPostRotation_validatesKeyTypeAndKey__SEC_G4_G5(t *testing.T) {
	r := chi.NewRouter()
	rotator := &fakeRotator{}
	MountRotations(r, rotator)

	for _, keyType := range []string{"", "CVK", "zpk"} {
		code, got := postRotation(t, r, testLinkKey, keyType)
		require.Equal(t, http.StatusBadRequest, code, keyType)
		require.Equal(t, problemTypeBase+"validation-error", got["type"], keyType)
	}
	code, got := postRotation(t, r, "not-a-uuid", keyZPK)
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, problemTypeBase+"insufficient-idempotency-key", got["type"])
	require.Zero(t, rotator.starts)
}

func TestPostRotation_replaysPerKeyAnd422OnADifferentBody__SEC_G4(t *testing.T) {
	r := chi.NewRouter()
	rotator := &fakeRotator{result: rotation.Row{ID: 5, KeyType: keyZPK, Status: rotationRunning}}
	MountRotations(r, rotator)

	_, first := postRotation(t, r, testLinkKey, keyZPK)
	code, replay := postRotation(t, r, testLinkKey, keyZPK)
	require.Equal(t, http.StatusAccepted, code)
	require.Equal(t, first["rotationId"], replay["rotationId"])
	require.Equal(t, 1, rotator.starts)

	code, got := postRotation(t, r, testLinkKey, "ZAK")
	require.Equal(t, http.StatusUnprocessableEntity, code)
	require.Equal(t, problemTypeBase+"idempotency-key-mismatch", got["type"])
}

func TestPostRotation_conflictWhileOneRuns__SEC_G2(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{startErr: rotation.ErrRotationInProgress})

	code, got := postRotation(t, r, testLinkKey, keyZPK)
	require.Equal(t, http.StatusConflict, code)
	require.Equal(t, problemTypeBase+"conflict", got["type"])
}

func TestGetRotation_unknownIsNotFound__SEC_G6(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{getErr: rotation.ErrNotFound})

	for _, id := range []string{"999999", "abc"} {
		code, got := serve(t, r, http.MethodGet, rotationsPath+"/"+id, "")
		require.Equal(t, http.StatusNotFound, code, id)
		require.Equal(t, problemTypeBase+"not-found", got["type"], id)
	}
}

func TestPostKeysAcquirerRotations_returns202WithKeyRotation__MCN_504_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{result: rotation.Row{ID: 1, KeyType: keyZPK, Status: "COMPLETED"}})

	body, _ := json.Marshal(map[string]string{"keyType": keyZPK})
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

	body, _ := json.Marshal(map[string]string{"keyType": keyZPK})
	req := httptest.NewRequest(http.MethodPost, "/v1/keys/acquirer/rotations", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetKeysAcquirerRotation_returns200__MCN_504_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{result: rotation.Row{ID: 7, KeyType: "ZAK", Status: rotationRunning}})

	req := httptest.NewRequest(http.MethodGet, "/v1/keys/acquirer/rotations/7", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"RUNNING"`)
}

func TestPostRotation_ZAKRotatesNowThatBothSidesReadTheActiveKey__SEC_G10(t *testing.T) {
	r := chi.NewRouter()
	rotator := &fakeRotator{result: rotation.Row{ID: 11, KeyType: "ZAK", Status: rotationRunning}}
	MountRotations(r, rotator)

	code, got := postRotation(t, r, testLinkKey, "ZAK")

	require.Equal(t, http.StatusAccepted, code, "the issuer reads its ACTIVE ZAK live (#122) and the gateway reloads on activation")
	require.Equal(t, "ZAK", got["keyType"])
	require.Equal(t, 1, rotator.starts)
}

func TestPostRotation_aReplayCarriesLocation__SEC_N4(t *testing.T) {
	r := chi.NewRouter()
	MountRotations(r, &fakeRotator{result: rotation.Row{ID: 9, KeyType: keyZPK, Status: rotationRunning}})

	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(struct {
			KeyType string `json:"keyType"`
		}{keyZPK})
		req := httptest.NewRequest(http.MethodPost, rotationsPath, bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", testLinkKey)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusAccepted, rec.Code)
		require.Equal(t, rotationsPath+"/9", rec.Header().Get("Location"), "attempt %d", i+1)
	}
}
