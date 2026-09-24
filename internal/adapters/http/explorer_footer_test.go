package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
)

// explorerBody fetches the rendered explorer page for one configured version.
func explorerBody(t *testing.T, opts httpapi.Options) string {
	t.Helper()
	srv := newTestServerWithOptions(t, seededQueryService(), opts)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

// The explorer is the first thing a started situs shows. Without a way out of
// it the OpenAPI spec and the Swagger UI exist but are unreachable: one has to
// know the paths to find them.
func TestExplorerFooter_VerlinktDieAPIDokumentation(t *testing.T) {
	body := explorerBody(t, httpapi.Options{Version: "v1.2.3"})

	if !strings.Contains(body, "<footer") {
		t.Fatal("die Seite hat kein <footer>-Element")
	}
	for _, want := range []string{`href="/docs"`, `href="/openapi"`, `href="/health/ready"`} {
		if !strings.Contains(body, want) {
			t.Errorf("der Footer verlinkt %s nicht", want)
		}
	}
}

// Every href in the footer must be a route this very server has registered.
// A footer that points at /openapi.json (tempus' path, not ours) would be a
// 404 shown on the landing page.
func TestExplorerFooter_VerlinktNurRegistrierteRouten(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{Version: "v1.2.3"})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()

	start := strings.Index(body, "<footer")
	if start < 0 {
		t.Fatal("die Seite hat kein <footer>-Element")
	}
	footer := body[start:]
	hrefs := regexp.MustCompile(`href="(/[^"]*)"`).FindAllStringSubmatch(footer, -1)
	if len(hrefs) == 0 {
		t.Fatal("der Footer enthaelt keine lokalen Verweise")
	}
	for _, h := range hrefs {
		probe := httptest.NewRecorder()
		srv.Router().ServeHTTP(probe, httptest.NewRequest(http.MethodGet, h[1], nil))
		if probe.Code == http.StatusNotFound {
			t.Errorf("der Footer verweist auf %s, das keine registrierte Route ist", h[1])
		}
	}
}

// The version is substituted at construction, not fetched: a footer that
// depends on /v1/info answering stays empty exactly when the index is the
// thing that is broken.
func TestExplorerFooter_ZeigtDieGebauteVersion(t *testing.T) {
	body := explorerBody(t, httpapi.Options{Version: "v0.13.0-12-gf1e0dc3"})

	if !strings.Contains(body, "v0.13.0-12-gf1e0dc3") {
		t.Error("die Seite nennt die gebaute Version nicht")
	}
	if strings.Contains(body, explorerVersionSentinel) {
		t.Errorf("der rohe Platzhalter %q steht noch in der Seite; die Ersetzung lief ins Leere", explorerVersionSentinel)
	}
}

// An unstamped build (go run, zero-value Options) must render a sensible
// footer rather than "situs " with a gap, and it must agree with what
// `situs version` calls such a build.
func TestExplorerFooter_FaelltAufDevZurueck(t *testing.T) {
	body := explorerBody(t, httpapi.Options{})

	if !strings.Contains(body, ">situs dev<") {
		t.Error("ohne eingestempelte Version zeigt der Footer nicht \"situs dev\"")
	}
}

// The code license and the data licenses are two different statements. situs
// is worth something because of the data, so the page names where it comes
// from, not only who wrote the code.
func TestExplorerFooter_NenntCopyrightUndDatenherkunft(t *testing.T) {
	body := explorerBody(t, httpapi.Options{Version: "v1.2.3"})

	for _, want := range []string{
		"© 2026 Jo Brunner",
		"MIT",
		"EEA EUNIS 2021",
		"EUNIS-ESy",
		"FloraVeg.EU",
		"CC BY 4.0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("der Footer nennt %q nicht", want)
		}
	}
}

// explorerVersionSentinel mirrors the production constant. The external test
// package cannot read it, and duplicating it here is deliberate: the internal
// sentinel test pins the asset side, this pins the rendered side.
const explorerVersionSentinel = "<!--situs:version-->"

// The version reaches the page as HTML text. It comes from the linker or the
// Docker VERSION build argument today, so nothing untrusted reaches it in
// practice — but Options.Version is an exported knob, and a value carrying
// markup would run for everyone opening "/". Escaping costs one call, so the
// page does not depend on every future caller being careful.
func TestExplorerFooter_EntschaerftMarkupInDerVersion(t *testing.T) {
	body := explorerBody(t, httpapi.Options{Version: `</p><script>alert(1)</script>`})

	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("die Version wird unescaped eingesetzt; Markup aus ihr landet ausfuehrbar in der Seite")
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("die Version steht nicht escaped in der Seite; sie muss als Text sichtbar bleiben, nicht verschwinden")
	}
}
