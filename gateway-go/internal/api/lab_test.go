package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestLabSamplesAndDecode_neverReturnPANPINBlockOrMACInClear__LAB_G1_G2(t *testing.T) {
	r := chi.NewRouter()
	MountLab(r)
	const clearPAN, pinBlock, mac = "9704360000004417", "7A3F09C21B84D6E0", "1C4E1D7B02C9A3F8"

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/lab/messages/samples", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var samples []decodeRequest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &samples))
	for _, leak := range []string{clearPAN, pinBlock, mac} {
		require.NotContains(t, rec.Body.String(), leak)
	}

	body, _ := json.Marshal(decodeRequest{Raw: samples[0].Raw})
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/lab/messages/decode", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, rec.Code)
	for _, leak := range []string{clearPAN, pinBlock, mac} {
		require.NotContains(t, rec.Body.String(), leak)
	}
	require.Contains(t, rec.Body.String(), `"raw":"16970436******4417"`)
}

func TestLabDecode_rawOverMaxLengthIsValidationError__LAB_G6(t *testing.T) {
	r := chi.NewRouter()
	MountLab(r)
	for name, size := range map[string]int{"just over maxLength": 8193, "far over": 1 << 20} {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(decodeRequest{Raw: strings.Repeat("0", size)})
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/lab/messages/decode", bytes.NewReader(body)))

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var got map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			require.Equal(t, "validation-error", got["type"])
		})
	}
}

type decodeRequest struct {
	Raw string `json:"raw"`
}
