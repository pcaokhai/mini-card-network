package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mcn/gateway-go/internal/iso8583"
	"github.com/mcn/gateway-go/internal/lab"
)

// maxLabRaw is the decode request's raw maxLength (contracts/openapi.yaml); the body limit adds
// room for the JSON envelope so an oversized raw is still read and answered precisely.
const (
	maxLabRaw  = 8192
	maxLabBody = maxLabRaw + 1024
)

// MountLab registers the Message Lab routes (contracts/openapi.yaml, tag "lab").
func MountLab(r chi.Router) {
	r.Post("/v1/lab/messages/decode", handleDecode)
	r.Post("/v1/lab/messages/encode", handleEncode)
	r.Get("/v1/lab/messages/samples", handleSamples)
}

func handleDecode(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Raw string `json:"raw"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxLabBody)).Decode(&body); err != nil {
		writeLabBodyProblem(w, err)
		return
	}
	if len(body.Raw) > maxLabRaw {
		problem(w, http.StatusBadRequest, "validation-error", "raw exceeds maxLength 8192")
		return
	}
	d, err := lab.Decode(body.Raw)
	if err != nil {
		writeCodecProblem(w, err)
		return
	}
	writeJSONBody(w, http.StatusOK, d)
}

func handleEncode(w http.ResponseWriter, req *http.Request) {
	var body struct {
		MTI    string            `json:"mti"`
		Fields map[string]string `json:"fields"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxLabBody)).Decode(&body); err != nil {
		writeLabBodyProblem(w, err)
		return
	}
	d, err := lab.Encode(body.MTI, body.Fields)
	if err != nil {
		writeCodecProblem(w, err)
		return
	}
	writeJSONBody(w, http.StatusOK, d)
}

func handleSamples(w http.ResponseWriter, _ *http.Request) {
	writeJSONBody(w, http.StatusOK, lab.Samples)
}

func writeLabBodyProblem(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		problem(w, http.StatusBadRequest, "validation-error", "request body exceeds the Lab message limit")
		return
	}
	problem(w, http.StatusBadRequest, "invalid-request", err.Error())
}

func writeCodecProblem(w http.ResponseWriter, err error) {
	var ce *iso8583.CodecError
	if iso8583.AsCodecError(err, &ce) {
		problem(w, http.StatusBadRequest, ce.Code, err.Error())
		return
	}
	problem(w, http.StatusBadRequest, "invalid-message", err.Error())
}

func writeJSONBody(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// problem writes an RFC 7807 problem+json body (docs/04 §2 error convention).
func problem(w http.ResponseWriter, status int, typ, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": typ, "title": typ, "status": status, "detail": detail})
}
