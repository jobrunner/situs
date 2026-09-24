package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
)

// Three of the sources situs redistributes say it outright in their own build
// scripts: "CC-BY-4.0 — any redistribution of derived data must retain
// attribution". The trait values reach users through
// GET /v1/species/{id}/traits, so naming only four of the nine sources is not
// a matter of taste. Each entry is checked with the license it actually
// carries — grouping EuroVegChecklist under CC BY 4.0 was wrong: FloraVeg.EU
// publishes it under its own terms of use, per pipelines/eurovegchecklist/manifest.yaml.
func TestExplorerFooter_NenntJedeQuelleMitIhrerLizenz(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()

	start := strings.Index(body, "<footer")
	if start < 0 {
		t.Fatal("die Seite hat kein <footer>-Element")
	}
	footer := body[start:]

	for _, want := range []string{
		"EEA EUNIS",        // EEA-Datenpolitik (ODC-BY)
		"EUNIS-ESy",        // Zenodo, CC BY 4.0
		"EuroVegChecklist", // FloraVeg.EU-Nutzungsbedingungen, NICHT CC BY
		"FloraVeg.EU",
		"EUR-Lex", // FFH-Anhang I, Beschluss 2011/833/EU
		"WGSRPD",  // TDWG
		"EIVE",    // Zenodo, CC BY 4.0
		"Tichý",   // Zenodo, CC BY 4.0
		"Midolo",  // Zenodo, CC BY 4.0
		"CC BY 4.0",
	} {
		if !strings.Contains(footer, want) {
			t.Errorf("der Footer nennt %q nicht", want)
		}
	}

	// Die Fehlzuordnung, die den Anlass gab: EuroVegChecklist darf nicht in
	// derselben Klammer wie die CC-BY-Quellen stehen.
	if strings.Contains(footer, "EuroVegChecklist · FloraVeg.EU (CC BY 4.0)") {
		t.Error("EuroVegChecklist steht weiterhin unter CC BY 4.0; FloraVeg.EU veroeffentlicht sie unter eigenen Nutzungsbedingungen")
	}
}

// axe-core measured the version line at 3.3:1 (light) and 4.34:1 (dark) —
// both below the 4.5:1 WCAG AA asks for at 12.8px. The cause was opacity:.75,
// carried over from ortus, which fades var(--muted) toward the background in
// both themes. Without it the same text measures 5.6:1 and 6.9:1.
func TestExplorerFooter_VersionszeileWirdNichtAusgeblichen(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	page := rec.Body.String()

	// Bewusst ohne den CSS-Zerleger aus explorer_viewport_test.go: der liegt
	// auf einem noch offenen Zweig, und ihn hier zu kopieren hiesse, ihn beim
	// Zusammenfuehren zweimal zu haben.
	const rule = "footer .version {"
	at := strings.Index(page, rule)
	if at < 0 {
		return // keine eigene Regel: dann bleicht auch nichts aus
	}
	decls := page[at+len(rule):]
	end := strings.Index(decls, "}")
	if end < 0 {
		t.Fatal("die Regel footer .version wird nicht geschlossen; das Stylesheet ist kaputt")
	}
	decls = decls[:end]
	if strings.Contains(decls, "opacity") {
		t.Errorf("footer .version setzt opacity (%q); gemessen faellt der Kontrast damit auf 3,3:1 (hell) und 4,34:1 (dunkel), noetig sind 4,5:1", strings.TrimSpace(decls))
	}
}
