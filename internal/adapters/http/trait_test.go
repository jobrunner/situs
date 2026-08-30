package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
	"github.com/jobrunner/situs/internal/ports/input"
)

func TestHandleSpeciesTraits_ReturnsSets(t *testing.T) {
	q := seededQueryService()
	q.traitSets = []input.TraitSetView{{Vocab: "eive", VocabVersion: "1.0",
		Values: []input.TraitValueView{{Dim: "M", Value: 4.2}}}}
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/species/wcvp:1/traits", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []input.TraitSetView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshaling body: %v", err)
	}
	if len(got) != 1 || got[0].Vocab != "eive" {
		t.Fatalf("got = %+v, want one eive set", got)
	}
	if q.gotTraitConceptID != "wcvp:1" {
		t.Errorf("gotTraitConceptID = %q, want wcvp:1", q.gotTraitConceptID)
	}
}

func TestHandleSpeciesTraits_PassesVocabQueryParam(t *testing.T) {
	q := seededQueryService()
	q.traitSets = []input.TraitSetView{}
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/species/wcvp:1/traits?vocab=eive", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.gotTraitVocab != "eive" {
		t.Errorf("gotTraitVocab = %q, want eive", q.gotTraitVocab)
	}
}

func TestHandleSpeciesTraits_UnknownVocabIsInvalidQuery(t *testing.T) {
	q := seededQueryService()
	q.traitErr = fmt.Errorf("trait vocabulary %q: %w", "bogus", input.ErrUnknownVocab)
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/species/wcvp:1/traits?vocab=bogus", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshaling body: %v", err)
	}
	if body["error"]["code"] != httpapi.CodeInvalidQuery {
		t.Errorf("error.code = %q, want %q", body["error"]["code"], httpapi.CodeInvalidQuery)
	}
}

func TestHandleSpeciesTraits_EmptyConceptIDIsInvalidQuery(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/species/%20/traits", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
