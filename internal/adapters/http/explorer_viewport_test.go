package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
)

// cssRules splits the page's <style> block into (selector, declarations)
// pairs. Crude on purpose — the stylesheet holds no at-rules with nested
// blocks except the one media query, whose inner rule this still yields.
func cssRules(t *testing.T, page string) map[string]string {
	t.Helper()
	open := strings.Index(page, "<style>")
	closeAt := strings.Index(page, "</style>")
	if open < 0 || closeAt < 0 {
		t.Fatal("die Seite hat keinen <style>-Block")
	}
	style := page[open+len("<style>") : closeAt]
	// Strip CSS comments, or a rule's explanation ends up in its selector and
	// the failure message becomes unreadable.
	for {
		from := strings.Index(style, "/*")
		to := strings.Index(style, "*/")
		if from < 0 || to < from {
			break
		}
		style = style[:from] + style[to+2:]
	}
	rules := map[string]string{}
	for _, block := range strings.Split(style, "}") {
		brace := strings.Index(block, "{")
		if brace < 0 {
			continue
		}
		sel := strings.Join(strings.Fields(block[:brace]), " ")
		rules[sel] = rules[sel] + block[brace+1:]
	}
	return rules
}

// ruleFor finds the one rule whose selector list contains every given part.
func ruleFor(t *testing.T, rules map[string]string, parts ...string) (string, string) {
	t.Helper()
	for sel, decls := range rules {
		all := true
		for _, p := range parts {
			if !strings.Contains(sel, p) {
				all = false
				break
			}
		}
		if all {
			return sel, decls
		}
	}
	return "", ""
}

// A <select> cannot be laid out narrower than its widest <option>: that text is
// its min-content width, and neither flex-shrink nor min-width on the select
// itself changes it. The typology filter carries
// "annex1 — Habitats Directive Annex I (2013…)" and measured 421 px, which
// forced the page to need 462 px. Chrome answers that on a phone by widening
// the layout viewport to 462 px and scaling the whole page to 84 %, so the
// 15 px body text arrives as 12.7 px and the 12.8 px labels as 10.8 px —
// everything unreadable, the drop-downs most visibly.
//
// The assertions bind each declaration to the selector it has to sit on. The
// earlier version searched the whole stylesheet for the two tokens and would
// have stayed green with max-width moved to an unrelated rule — which is the
// one way this regression actually comes back.
func TestExplorerPage_FormularelementeSprengenDenViewportNicht(t *testing.T) {
	srv := newTestServerWithOptions(t, seededQueryService(), httpapi.Options{})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	rules := cssRules(t, rec.Body.String())

	sel, decls := ruleFor(t, rules, "input", "select", "textarea", "button")
	if sel == "" {
		t.Fatal("keine Regel fuer die Formularelemente gefunden")
	}
	if !strings.Contains(decls, "max-width:100%") {
		t.Errorf("die Regel %q begrenzt die Breite nicht (max-width:100%%); ein select waechst auf die Breite seiner laengsten Option und sprengt den Viewport", sel)
	}

	sel, decls = ruleFor(t, rules, ".globals > div", ".row > div")
	if sel == "" {
		t.Fatal("keine Regel fuer die Flex-Kinder von .globals und .row gefunden")
	}
	if !strings.Contains(decls, "min-width:0") {
		t.Errorf("die Regel %q setzt kein min-width:0; ein Flex-Item mit min-width:auto schrumpft nie unter seine min-content-Breite, und max-width:100%% bleibt damit wirkungslos", sel)
	}
}
