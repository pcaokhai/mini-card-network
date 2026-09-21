package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/go-chi/chi/v5"
)

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../../contracts/openapi.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	// Not calling doc.Validate(): contracts/openapi.yaml's Journey.money[].delta description has
	// an unquoted comma in a YAML flow mapping ("description: signed minor units, 0 for balance
	// rows"), which every YAML parser reads as a stray extra key. That schema is unrelated to the
	// Lab API and contracts/ is read-only for this story (MCN-103) — report and fix separately.
	// Response validation below only needs the resolved schema tree, not full spec-level Validate.
	return doc
}

func TestLabResponses_matchOpenAPISpec__MCN_103_AC4(t *testing.T) {
	doc := loadSpec(t)
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("build router: %v", err)
	}

	r := chi.NewRouter()
	MountLab(r)

	cases := []*http.Request{
		httptest.NewRequest(http.MethodGet, "http://localhost:8080/v1/lab/messages/samples", nil),
		httptest.NewRequest(http.MethodPost, "http://localhost:8080/v1/lab/messages/decode",
			bytes.NewReader(mustJSON(map[string]string{"raw": "0800822000000000000004000000000000000921073300000200301"}))),
	}
	for _, req := range cases {
		req.Header.Set("Content-Type", "application/json")
		route, pathParams, err := router.FindRoute(req)
		if err != nil {
			t.Fatalf("find route for %s: %v", req.URL.Path, err)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		responseValidationInput := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: pathParams, Route: route},
			Status:                 rec.Code,
			Header:                 rec.Header(),
		}
		responseValidationInput.SetBodyBytes(rec.Body.Bytes())
		if err := openapi3filter.ValidateResponse(context.Background(), responseValidationInput); err != nil {
			t.Fatalf("response for %s does not match spec: %v", req.URL.Path, err)
		}
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
