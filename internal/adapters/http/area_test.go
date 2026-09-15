package httpapi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// GET /v1/areas is the discovery entry point for the ?area= filter: without
// it a client has to know the WGSRPD code table by heart and cannot tell
// which of its codes this index can answer at all.
func TestAreasListsWhatTheFilterCanAnswer(t *testing.T) {
	rec := serve(t, newTestServer(t, seededQueryService()), http.MethodGet, "/v1/areas")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []struct {
		Scheme string `json:"scheme"`
		Code   string `json:"code"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %q: %v", rec.Body, err)
	}
	if len(got) != 2 {
		t.Fatalf("areas = %+v, want 2 entries", got)
	}
	if got[0].Code != "FRA" || got[0].Name != "France" || got[0].Scheme != domain.SchemeWGSRPDL3 {
		t.Errorf("areas[0] = %+v, want FRA/France in %s", got[0], domain.SchemeWGSRPDL3)
	}
	if got[1].Code != "GER" {
		t.Errorf("areas[1].code = %q, want GER (sorted by code)", got[1].Code)
	}
}

// An index with no distribution data answers [], never null: a client ranges
// over the list unconditionally to build its area selector.
func TestAreasIsAlwaysAnArrayNeverNull(t *testing.T) {
	q := seededQueryService()
	q.areas = []input.AreaView{}

	rec := serve(t, newTestServer(t, q), http.MethodGet, "/v1/areas")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("body = %q, want the literal empty array [], never null", body)
	}
}

func TestAreasFailsLoudlyOnRepositoryError(t *testing.T) {
	q := seededQueryService()
	q.areasErr = errors.New("index unreadable")

	rec := serve(t, newTestServer(t, q), http.MethodGet, "/v1/areas")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Errorf("body = %q, want the INTERNAL_ERROR envelope", rec.Body)
	}
}
