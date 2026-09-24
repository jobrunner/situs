package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
)

// A <select> cannot be laid out narrower than its widest <option>: that text is
// its min-content width, and neither flex-shrink nor min-width on the select
// itself changes it. The typology filter carries
// "annex1 — Habitats Directive Annex I (2013…)" and measured 421 px, which
// forced the page to need 462 px. Chrome answers that on a phone by widening
// the layout viewport to 462 px and scaling the whole page to 84 %, so the
// 15 px body text arrives as 12.7 px and the 12.8 px labels as 10.8 px —
// everything unreadable, the drop-downs most visibly.
//
// Two rules are needed and measured, not one: max-width alone left the page at
// 462 px, because the flex item around the select keeps min-width:auto and so
// never shrinks below its own min-content width — "100 %" of an unshrunk parent
// is still 421 px. With both, the layout viewport measured 393 px, exactly the
// device width, and no scaling at all.
func TestExplorerPage_FormularelementeSprengenDenViewportNicht(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()

	style := body[strings.Index(body, "<style>"):strings.Index(body, "</style>")]

	if !strings.Contains(style, "max-width:100%") {
		t.Error("kein max-width:100% im Stylesheet; ein select waechst auf die Breite seiner laengsten Option und sprengt den Viewport")
	}
	if !strings.Contains(style, "min-width:0") {
		t.Error("kein min-width:0 im Stylesheet; die Flex-Kinder schrumpfen nicht unter ihre min-content-Breite, und max-width:100% bleibt damit wirkungslos")
	}
}
