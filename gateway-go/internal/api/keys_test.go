package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeKeyLister struct{ rows []store.KeyRow }

func (f *fakeKeyLister) ListCurrent(context.Context) ([]store.KeyRow, error) { return f.rows, nil }

func TestGetKeysAcquirer_returnsKeyInfoWithNoClearKeyField__MCN_501_AC1_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountKeys(r, &fakeKeyLister{rows: []store.KeyRow{{KeyType: keyZPK, KCV: "AABBCC", Status: keyActive}}}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/keys/acquirer", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"kcv":"AABBCC"`)
	require.NotContains(t, rec.Body.String(), "keyUnderLmk")
	require.NotContains(t, rec.Body.String(), "KeyUnderLMKHex")
}

func TestGetKeysAcquirer_usesThePerTypeLifetimePolicy__SEC_G9(t *testing.T) {
	activated := time.Now().Add(-10 * 24 * time.Hour)
	r := chi.NewRouter()
	MountKeys(r, &fakeKeyLister{rows: []store.KeyRow{
		{KeyType: keyZPK, KCV: "AABBCC", Status: keyActive, ActivatedAt: &activated},
		{KeyType: "ZMK", KCV: "DDEEFF", Status: keyActive, ActivatedAt: &activated},
	}}, map[string]int{"ZPK": 30})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/keys/acquirer", nil))

	var got []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.EqualValues(t, 30, got[0]["lifetimeDays"])
	require.EqualValues(t, 20, got[0]["daysRemaining"])
	require.EqualValues(t, 365, got[1]["lifetimeDays"], "a type without a policy keeps the default")
}
