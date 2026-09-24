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

	// Die Zuordnung Quelle -> Lizenz ist der Punkt, nicht die blosse Nennung:
	// ein Footer, der die EEA-Zeile unter CC BY 4.0 fuehrt, waere falsch und
	// stuende trotzdem voller richtiger Namen. Geprueft werden deshalb die
	// ganzen Klauseln. Der Footer bricht sie ueber mehrere Zeilen um, also wird
	// vorher der Weissraum vereinheitlicht.
	normalized := strings.Join(strings.Fields(footer), " ")

	for _, klausel := range []string{
		"EEA EUNIS 2021 (EEA-Datenpolitik)",
		"EuroVegChecklist / FloraVeg.EU (Nutzungsbedingungen)",
		"FFH-Anhang I (EUR-Lex)",
		"WGSRPD (TDWG)",
		// CC BY 4.0 verlangt die Nennung der Urheber, nicht nur des
		// Datensatznamens; die Pipelines formulieren das als Zitierpflicht.
		"Je CC BY 4.0, mit Zitierpflicht:",
		"EUNIS-ESy (Chytrý et al. 2020)",
		"EVC-Verbreitungskarten (Preislerová et al. 2022, 2024)",
		"Habitat-Factsheets (Chytrý et al. 2020)",
		"EIVE 1.0 (Dengler et al. 2023)",
		"Zeigerwerte (Tichý et al. 2023)",
		"Störungszeiger (Midolo et al. 2023)",
	} {
		if !strings.Contains(normalized, klausel) {
			t.Errorf("der Footer fuehrt die Klausel %q nicht; entweder fehlt die Quelle oder ihre Lizenz ist anders zugeordnet", klausel)
		}
	}

	// Die Fehlzuordnung, die den Anlass gab: die EuroVegChecklist stand in
	// derselben Klammer wie die CC-BY-Quellen.
	if strings.Contains(normalized, "EuroVegChecklist · FloraVeg.EU (CC BY 4.0)") {
		t.Error("EuroVegChecklist steht weiterhin unter CC BY 4.0; FloraVeg.EU veroeffentlicht sie unter eigenen Nutzungsbedingungen")
	}
}

// axe-core measured the version line at 3.3:1 (light) and 4.34:1 (dark) —
// both below the 4.5:1 WCAG AA asks for at 12.8px. The cause was opacity:.75,
// carried over from ortus, which fades var(--muted) toward the background in
// both themes. Without it the same text measures 5.64:1 and 6.74:1.
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
		// Nicht zurueckkehren: ein Test, der sich stilllegt, sobald sein
		// Pruefgegenstand fehlt, prueft im entscheidenden Moment nichts. Die
		// eigene Regel ist ausserdem die einzige Stelle, an der die
		// Versionszeile ueberhaupt auf opacity geprueft wird.
		t.Fatal("die Regel footer .version fehlt; ohne sie prueft dieser Test die Kontrastanforderung gar nicht")
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
