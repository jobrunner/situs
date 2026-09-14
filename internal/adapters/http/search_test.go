package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestSearchRoute_ReturnsHitsWithExplicitNullConceptID(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=fagus", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	// The null must survive as a key, not vanish: clients distinguish
	// "unresolvable" from "field absent".
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"concept_id":null`)) {
		t.Errorf("body = %s, want an explicit \"concept_id\":null for the unresolved hit", rec.Body)
	}

	var hits []struct {
		VerbatimName string  `json:"verbatim_name"`
		ConceptID    *string `json:"concept_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &hits); err != nil {
		t.Fatalf("unmarshaling: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits; the test server's index must carry at least one Fagus name")
	}
}

func TestSearchRoute_MissingQueryIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a missing q", rec.Code)
	}
}

func TestSearchRoute_UnparsableLimitIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=fagus&limit=viele", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unparsable limit", rec.Code)
	}
}

func TestSearchRoute_ExplicitZeroLimitIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=fagus&limit=0", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an explicit limit=0; a client that mistypes limit must not silently get the default", rec.Code)
	}
}

func TestSearchRoute_ExplicitEmptyLimitIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=fagus&limit=", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an explicit but empty limit; it must not be mistaken for an absent one", rec.Code)
	}
}

func TestSearchRoute_OversizedLimitIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=fagus&limit=101", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a limit above the maximum", rec.Code)
	}
}

func TestSearchRoute_NoMatchIsEmptyArrayNotNull(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/search?q=zzzznothing", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("body = %q, want %q", got, "[]")
	}
}
