package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestLabDecode__MCN_103_AC1(t *testing.T) {
	r := chi.NewRouter()
	MountLab(r)
	body, _ := json.Marshal(map[string]string{"raw": "0800822000000000000004000000000000000921073300000200301"})
	req := httptest.NewRequest(http.MethodPost, "/v1/lab/messages/decode", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "0800", got["mti"])
}

func TestLabSamples_returnsFour__MCN_103_AC3(t *testing.T) {
	r := chi.NewRouter()
	MountLab(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/lab/messages/samples", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var got []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 4)
}

func TestLabDecode_invalidRawReturnsProblem(t *testing.T) {
	r := chi.NewRouter()
	MountLab(r)
	body, _ := json.Marshal(map[string]string{"raw": "not-hex-or-digits"})
	req := httptest.NewRequest(http.MethodPost, "/v1/lab/messages/decode", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusOK, rec.Code)
}

func TestLabEncode__MCN_103_AC2(t *testing.T) {
	r := chi.NewRouter()
	MountLab(r)
	body, _ := json.Marshal(map[string]any{"mti": "0800", "fields": map[string]string{"7": "0921073300", "11": "000200", "70": "301"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/lab/messages/encode", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "0800", got["mti"])
}
