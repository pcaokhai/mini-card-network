package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

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
		writeLabBodyProblem(w, req, err)
		return
	}
	if len(body.Raw) > maxLabRaw {
		validationProblem(w, req, fieldErrors{{Field: "raw", Message: "exceeds maxLength 8192"}})
		return
	}
	d, err := lab.Decode(body.Raw)
	if err != nil {
		writeCodecProblem(w, req, err)
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
		writeLabBodyProblem(w, req, err)
		return
	}
	d, err := lab.Encode(body.MTI, body.Fields)
	if err != nil {
		writeCodecProblem(w, req, err)
		return
	}
	writeJSONBody(w, http.StatusOK, d)
}

func handleSamples(w http.ResponseWriter, _ *http.Request) {
	writeJSONBody(w, http.StatusOK, lab.Samples)
}

func writeLabBodyProblem(w http.ResponseWriter, req *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		validationProblem(w, req, fieldErrors{{Field: fieldBody, Message: "exceeds the Lab message limit"}})
		return
	}
	validationProblem(w, req, fieldErrors{{Field: fieldBody, Message: "must be a JSON object matching the schema"}})
}

// writeCodecProblem answers a message the codec rejects as a validation-error naming the codec's
// error code (INVALID_MTI, UNKNOWN_FIELD, ...) in errors[] (LAB-G4).
func writeCodecProblem(w http.ResponseWriter, req *http.Request, err error) {
	validationProblem(w, req, fieldErrors{{Field: "raw", Message: err.Error()}})
}

func writeJSONBody(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
