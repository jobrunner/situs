package httpapi_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/ports/input"
)

func postMatch(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/habitat-types/match", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(rec, req)
	return rec
}

func TestMatchRoute_LiefertRangliste(t *testing.T) {
	rec := postMatch(t, `{"concept_ids":["wcvp:concept:1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"input"`, `"matches"`, `"score"`, `"matched"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("Antwort enthaelt %s nicht: %s", want, rec.Body.String())
		}
	}
}

// Nach nichts zu fragen ist ein Fehler des Aufrufers, keine Frage.
func TestMatchRoute_LeereListeIstInvalidQuery(t *testing.T) {
	rec := postMatch(t, `{"concept_ids":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "INVALID_QUERY") {
		t.Errorf("Fehlerhuelle fehlt: %s", rec.Body.String())
	}
}

// Ein unbekanntes Gebiet darf nicht stillschweigend ignoriert werden. Die
// Pruefung selbst liegt im Use-Case — dort ist der Repository-Zugang, und dort
// ist sie getestet. Der Handler hat hier genau eine Aufgabe: den Fehler als
// INVALID_QUERY herausgeben statt als 500.
func TestMatchRoute_UnbekanntesGebietIstInvalidQuery(t *testing.T) {
	q := seededQueryService()
	q.matchErr = fmt.Errorf("area %q: %w", "XXX", input.ErrUnknownArea)
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/habitat-types/match",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1"],"area":"XXX"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "INVALID_QUERY") {
		t.Errorf("Fehlerhuelle fehlt: %s", rec.Body.String())
	}
}

// Die area-Angabe muss den Use-Case auch wirklich erreichen.
func TestMatchRoute_ReichtDasGebietDurch(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/habitat-types/match",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1"],"area":"GER"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(rec, req)

	if q.matchRequest.Area != "GER" {
		t.Errorf("area kam als %q im Use-Case an, erwartet GER", q.matchRequest.Area)
	}
}

func TestMatchRoute_LimitAusserhalbDerGrenzen(t *testing.T) {
	for _, body := range []string{
		`{"concept_ids":["wcvp:concept:1"],"limit":0}`,
		`{"concept_ids":["wcvp:concept:1"],"limit":51}`,
		`{"concept_ids":["wcvp:concept:1"],"limit":-1}`,
	} {
		if rec := postMatch(t, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s -> Status %d, want 400", body, rec.Code)
		}
	}
}

// Ein altes Feld darf eine klare 400 bekommen, keine leere Antwort.
func TestMatchRoute_UnbekanntesFeldIstInvalidQuery(t *testing.T) {
	if rec := postMatch(t, `{"names":["Fagus sylvatica"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400", rec.Code)
	}
}

// Nur POST. Die Vertragspruefung verlangt ausserdem, dass die Route in beiden
// OpenAPI-Kopien steht — das prueft TestRoutesMatchOpenAPISpec.
func TestMatchRoute_NurPost(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/habitat-types/match", nil))
	if rec.Code == http.StatusOK {
		t.Error("GET auf die Match-Route antwortet 200")
	}
}

// Dieselben Pruefungen wie beim Schwester-Endpunkt: der Dienst darf sich in
// zwei Routen nicht widersprechen.
func TestMatchRoute_PrueftDenBodyWieDerBatch(t *testing.T) {
	t.Run("leerer Eintrag", func(t *testing.T) {
		rec := postMatch(t, `{"concept_ids":["  "]}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("Status = %d, want 400: ein leerer String ist kein Backbone", rec.Code)
		}
	})
	t.Run("zweites JSON-Objekt", func(t *testing.T) {
		rec := postMatch(t, `{"concept_ids":["wcvp:concept:1"]}{"concept_ids":["wcvp:concept:2"]}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("Status = %d, want 400: das zweite Objekt wuerde stillschweigend verschwinden", rec.Code)
		}
	})
	t.Run("zu viele Eintraege", func(t *testing.T) {
		ids := make([]string, 301)
		for i := range ids {
			ids[i] = `"wcvp:concept:1"`
		}
		rec := postMatch(t, `{"concept_ids":[`+strings.Join(ids, ",")+`]}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("Status = %d, want 400 bei 301 Eintraegen", rec.Code)
		}
	})
}

// Eine unbekannte Typologie ist ein Tippfehler des Aufrufers, kein leeres
// Ergebnis — sonst ist "quatsch@1" nicht von "diese Arten passen nirgends"
// zu unterscheiden. Dieselbe Haltung wie beim Gebiet.
func TestMatchRoute_UnbekannteTypologieIstInvalidQuery(t *testing.T) {
	q := seededQueryService()
	q.matchErr = fmt.Errorf("typology %q: %w", "quatsch@1", input.ErrUnknownTypology)
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/habitat-types/match",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1"],"typology":"quatsch@1"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}
