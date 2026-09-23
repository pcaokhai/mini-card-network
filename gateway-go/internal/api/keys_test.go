package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mcn/gateway-go/internal/store"
)

type fakeKeyLister struct{ rows []store.KeyRow }

func (f *fakeKeyLister) List(context.Context) ([]store.KeyRow, error) { return f.rows, nil }

func TestGetKeysAcquirer_returnsKeyInfoWithNoClearKeyField__MCN_501_AC1_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountKeys(r, &fakeKeyLister{rows: []store.KeyRow{{KeyType: "ZPK", KCV: "AABBCC", Status: "ACTIVE"}}})

	req := httptest.NewRequest(http.MethodGet, "/v1/keys/acquirer", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"kcv":"AABBCC"`)
	require.NotContains(t, rec.Body.String(), "keyUnderLmk")
	require.NotContains(t, rec.Body.String(), "KeyUnderLMKHex")
}
