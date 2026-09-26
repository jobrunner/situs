package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplorerPage_HatDasMatchPanel(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`id="match"`,
		`"/v1/habitat-types/match"`,
		`id="match-ids"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Seite enthaelt %q nicht; das Match-Panel fehlt", want)
		}
	}
}

// Die Seite sagt im Untertitel, sie rechne nichts selbst aus. Das bleibt wahr:
// die Rangliste kommt vom Dienst. Der Untertitel muss das trotzdem einordnen,
// sonst widerspricht er dem neuen Panel.
func TestExplorerPage_UntertitelOrdnetDieRanglisteEin(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	if strings.Contains(body, "Es wird nichts berechnet oder zusammengefasst, was der Dienst") &&
		!strings.Contains(body, "Rangliste") {
		t.Error("der Untertitel schliesst jede Berechnung aus, das Match-Panel zeigt aber eine Rangliste des Dienstes")
	}
}
