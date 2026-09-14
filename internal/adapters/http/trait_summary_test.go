package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTraitSummaryRoute_AnswersWithPerVocabularyStatistics(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	body := strings.NewReader(`{"concept_ids":["wcvp:concept:83891"]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"vocabularies"`) {
		t.Errorf("body = %s, want a vocabularies object", rec.Body)
	}
}

func TestTraitSummaryRoute_EmptyListIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary",
		strings.NewReader(`{"concept_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestTraitSummaryRoute_MalformedBodyIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary",
		strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// Two concatenated objects must not decode as the first one silently.
func TestTraitSummaryRoute_TrailingJSONIs400(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1"]}{"concept_ids":["wcvp:concept:2"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for trailing JSON", rec.Code)
	}
}

func TestTraitSummaryRoute_UnknownConceptIs200WithReason(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	req := httptest.NewRequest(http.MethodPost, "/v1/species/traits/summary",
		strings.NewReader(`{"concept_ids":["wcvp:concept:999999999"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — an unknown id is a normal answer", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unknown_concept") {
		t.Errorf("body = %s, want the unknown_concept reason", rec.Body)
	}
}
