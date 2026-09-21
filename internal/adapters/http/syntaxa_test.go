package httpapi_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
	"github.com/jobrunner/situs/internal/ports/input"
)

var errFakeIndex = errors.New("fake index failure")

func getSyntaxonJSON(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v (body %q)", path, err, rec.Body.String())
	}
	return rec.Code, body
}

// The embedded SyntaxonRef fields have to sit FLAT in the same object — that is
// how Go serializes embedded structs, and exactly what the OpenAPI allOf
// describes. A nested object here would have schema and wire claim different
// things.
func TestSyntaxon_EingebetteteFelderStehenFlachImSelbenObjekt(t *testing.T) {
	code, body := getSyntaxonJSON(t, "/v1/syntaxon/BRO-01A")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	for _, key := range []string{
		"id", "rank", "name", "author", "parent_id", "alt_code", "source",
		"parent_provenance", "life_form_group", "ancestors", "children",
		"direct_habitat_type_count",
	} {
		if _, ok := body[key]; !ok {
			t.Errorf("Feld %q fehlt auf der ersten Ebene", key)
		}
	}
	if _, nested := body["SyntaxonRef"]; nested {
		t.Error("SyntaxonRef erscheint als verschachteltes Objekt; es muss promoviert werden")
	}
	if body["id"] != "BRO-01A" {
		t.Errorf("id = %v, erwartet BRO-01A", body["id"])
	}
}

func TestSyntaxon_AhnenpfadIstAeussersteZuerst(t *testing.T) {
	_, body := getSyntaxonJSON(t, "/v1/syntaxon/BRO-01A")
	ancestors, ok := body["ancestors"].([]any)
	if !ok || len(ancestors) != 3 {
		t.Fatalf("ancestors = %v, erwartet drei Einträge", body["ancestors"])
	}
	want := []string{"C", "CA", "CA01"}
	for i, a := range ancestors {
		got := a.(map[string]any)["id"]
		if got != want[i] {
			t.Errorf("ancestors[%d].id = %v, erwartet %q", i, got, want[i])
		}
	}
}

// children and ancestors carry NO omitempty: a missing field and an empty list
// are not the same statement for a client.
func TestSyntaxon_LeereListenBleibenImJSONSichtbar(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxon/BRO-01A", nil))

	if !strings.Contains(rec.Body.String(), `"children":[]`) {
		t.Errorf("Antwort enthaelt kein leeres children-Array: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxon/C", nil))
	if !strings.Contains(rec.Body.String(), `"ancestors":[]`) {
		t.Errorf("Antwort einer Formation enthaelt kein leeres ancestors-Array: %s", rec.Body.String())
	}
}

func TestSyntaxon_FeldheisstDirectHabitatTypeCount(t *testing.T) {
	_, body := getSyntaxonJSON(t, "/v1/syntaxon/BRO-01A")
	if _, ok := body["habitat_type_count"]; ok {
		t.Error("habitat_type_count erscheint; das Feld heisst direct_habitat_type_count")
	}
	if got := body["direct_habitat_type_count"]; got != float64(1) {
		t.Errorf("direct_habitat_type_count = %v, erwartet 1", got)
	}
}

func TestSyntaxon_UnbekannteIDIst404(t *testing.T) {
	code, body := getSyntaxonJSON(t, "/v1/syntaxon/GIBTSNICHT")
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	env := body["error"].(map[string]any)
	if env["code"] != httpapi.CodeNotFound {
		t.Errorf("code = %v, erwartet NOT_FOUND", env["code"])
	}
}

// A path segment is not a query: a blank or whitespace-only {id} is 404, not
// INVALID_QUERY.
func TestSyntaxon_LeerraumPfadsegmentIst404(t *testing.T) {
	code, body := getSyntaxonJSON(t, "/v1/syntaxon/%20")
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	env := body["error"].(map[string]any)
	if env["code"] != httpapi.CodeNotFound {
		t.Errorf("code = %v, erwartet NOT_FOUND", env["code"])
	}
}

// ?lang= is passed through and answers in English until German syntaxa names
// get their own round (design, section 10) — the state of the data, not a
// special rule.
func TestSyntaxon_LangWirdAkzeptiertUndAntwortetEnglisch(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxon/BRO-01A?lang=de", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.lang != "de" {
		t.Errorf("durchgereichte Sprache = %q, erwartet de", q.lang)
	}
	if !strings.Contains(rec.Body.String(), "Bromion erecti") {
		t.Error("der englische Name fehlt; er bleibt die Identitaet")
	}
}

// An omitted ?rank= means the formations, not all 1882 rows. The default lives
// in the handler; the port knows no empty rank.
func TestSyntaxa_OhneRankSindDieFormationen(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxa", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if q.gotRank != "formation" {
		t.Errorf("durchgereichter Rang = %q, erwartet formation", q.gotRank)
	}
	var refs []input.SyntaxonRef
	if err := json.Unmarshal(rec.Body.Bytes(), &refs); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(refs) != 1 || refs[0].ID != "C" {
		t.Errorf("Antwort = %+v, erwartet die Formation C", refs)
	}
}

func TestSyntaxa_ReichtDieLebensformGruppeWeiter(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/v1/syntaxa?life_form_group=bryophyte_lichen", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if q.gotLifeFormGroup != "bryophyte_lichen" {
		t.Errorf("durchgereichte Gruppe = %q, erwartet bryophyte_lichen", q.gotLifeFormGroup)
	}
}

func TestSyntaxa_UnbekannterRangIst400MitDenErlaubtenWerten(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxa?rank=association", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	env := body["error"].(map[string]any)
	if env["code"] != httpapi.CodeInvalidQuery {
		t.Errorf("code = %v, erwartet INVALID_QUERY", env["code"])
	}
	msg, _ := env["message"].(string)
	for _, want := range []string{"alliance", "class", "formation", "order"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Nachricht %q nennt den erlaubten Wert %q nicht", msg, want)
		}
	}
}

// The group is the one value set that is wired in (it stands as a CHECK in the
// schema). A typo must not reach the port at all.
func TestSyntaxa_UnbekannteLebensformGruppeIst400UndErreichtDenPortNicht(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxa?life_form_group=pilze", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if q.rankCalls != 0 {
		t.Errorf("SyntaxaByRank wurde %d mal gerufen, erwartet 0", q.rankCalls)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	msg := body["error"].(map[string]any)["message"].(string)
	for _, want := range []string{"phanerogam", "bryophyte_lichen", "algae"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Nachricht %q nennt den erlaubten Wert %q nicht", msg, want)
		}
	}
}

// A dangling parent_id is the one inconsistency only this route can surface:
// the application layer reports it as an index inconsistency (f.err), and that
// has to answer INTERNAL_ERROR, never NOT_FOUND — "the id does not exist" and
// "the index is broken" are different statements to a client.
func TestSyntaxon_UnerwarteterFehlerIst500MitInternalError(t *testing.T) {
	q := seededQueryService()
	q.err = errFakeIndex
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxon/BRO-01A", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	env := body["error"].(map[string]any)
	if env["code"] != httpapi.CodeInternalError {
		t.Errorf("code = %v, erwartet INTERNAL_ERROR", env["code"])
	}
}

// The JSON shape is the contract: "distribution" present with two empty
// arrays means "checked, occurs nowhere"; absent means "nobody looked".
func TestSyntaxonJSONTraegtVerbreitungMitLeerenListen(t *testing.T) {
	_, body := getSyntaxonJSON(t, "/v1/syntaxon/CA01B")
	dist, ok := body["distribution"].(map[string]any)
	if !ok {
		t.Fatalf("distribution fehlt oder ist kein Objekt: %v", body["distribution"])
	}
	if dist["area_scheme"] != "evc_territory" {
		t.Errorf("area_scheme = %v", dist["area_scheme"])
	}
	for _, field := range []string{"verified", "uncertain"} {
		list, ok := dist[field].([]any)
		if !ok {
			t.Errorf("%s ist kein Array: %v", field, dist[field])
			continue
		}
		if len(list) != 0 {
			t.Errorf("%s = %v, erwartet leer", field, list)
		}
	}
}

func TestSyntaxonJSONLaesstDistributionOhneAbdeckungWeg(t *testing.T) {
	_, body := getSyntaxonJSON(t, "/v1/syntaxon/RA01A")
	if _, ok := body["distribution"]; ok {
		t.Errorf("distribution erscheint, obwohl keine Aussage vorliegt: %v", body["distribution"])
	}
}

func TestSyntaxa_FehlerDesIndexIst500(t *testing.T) {
	q := seededQueryService()
	q.syntaxaErr = errFakeIndex
	srv := newTestServer(t, q)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxa", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// get issues a GET against the server's router and returns the recorder — the
// same pattern getSyntaxonJSON above already uses, just without decoding.
func get(t *testing.T, srv *httpapi.Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func getJSONArray(t *testing.T, srv *httpapi.Server, path string) []any {
	t.Helper()
	rec := get(t, srv, path)
	var body []any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v (body %q)", path, err, rec.Body.String())
	}
	return body
}

func TestSyntaxaAreaFilterErreichtDenPort(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	get(t, srv, "/v1/syntaxa?rank=alliance&area=austria-alps&include=uncertain,verified")
	want := input.SyntaxonAreaFilter{Code: "austria-alps",
		Include: []string{"uncertain", "verified"}}
	if !reflect.DeepEqual(q.syntaxonAreaFilter, want) {
		t.Errorf("Filter = %+v, erwartet %+v", q.syntaxonAreaFilter, want)
	}
}

func TestSyntaxaAreaOhneIncludeIstVerified(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)
	get(t, srv, "/v1/syntaxa?rank=alliance&area=austria-alps")
	if !reflect.DeepEqual(q.syntaxonAreaFilter.Include, []string{"verified"}) {
		t.Errorf("Include = %v, erwartet [verified]", q.syntaxonAreaFilter.Include)
	}
}

func TestSyntaxaAbgelehnteFilterkombinationen(t *testing.T) {
	srv := newTestServer(t, seededQueryService())
	for name, tc := range map[string]struct{ path, mentions string }{
		"include ohne area":       {"/v1/syntaxa?rank=alliance&include=verified", "area"},
		"unbekanntes include":     {"/v1/syntaxa?rank=alliance&area=austria-alps&include=probable", "probable"},
		"leeres include":          {"/v1/syntaxa?rank=alliance&area=austria-alps&include=", "include"},
		"leeres Element":          {"/v1/syntaxa?rank=alliance&area=austria-alps&include=verified,,uncertain", "include"},
		"include zweimal":         {"/v1/syntaxa?rank=alliance&area=austria-alps&include=verified&include=uncertain", "include"},
		"area mit rank=formation": {"/v1/syntaxa?rank=formation&area=austria-alps", "rank"},
		"area ohne rank":          {"/v1/syntaxa?area=austria-alps", "rank"},
	} {
		t.Run(name, func(t *testing.T) {
			res := get(t, srv, tc.path)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("Status %d, erwartet 400", res.Code)
			}
			body := res.Body.String()
			if !strings.Contains(body, httpapi.CodeInvalidQuery) {
				t.Errorf("Antwort ohne INVALID_QUERY: %s", body)
			}
			if !strings.Contains(body, tc.mentions) {
				t.Errorf("Antwort nennt %q nicht: %s", tc.mentions, body)
			}
		})
	}
}

func TestSyntaxaUnbekanntesGebietIstInvalidQuery(t *testing.T) {
	q := seededQueryService()
	q.syntaxaErr = fmt.Errorf("area %q: %w", "gibtsnicht", input.ErrUnknownArea)
	srv := newTestServer(t, q)
	res := get(t, srv, "/v1/syntaxa?rank=alliance&area=gibtsnicht")
	if res.Code != http.StatusBadRequest {
		t.Errorf("Status %d, erwartet 400", res.Code)
	}
}

func TestSyntaxaJSONZeigtOccurrenceUndLaesstEsWeg(t *testing.T) {
	// The three-valuedness on the wire: a marked hit and a carried
	// unjudgeable row in the SAME list, distinguishable only by the field's
	// presence.
	srv := newTestServer(t, seededQueryService())
	body := getJSONArray(t, srv, "/v1/syntaxa?rank=alliance&area=austria-alps")
	byID := map[string]map[string]any{}
	for _, entry := range body {
		e := entry.(map[string]any)
		byID[e["id"].(string)] = e
	}
	if got := byID["CA01A"]["occurrence"]; got != "verified" {
		t.Errorf("CA01A.occurrence = %v, erwartet verified", got)
	}
	if _, ok := byID["RA01A"]["occurrence"]; ok {
		t.Error("RA01A traegt occurrence, obwohl keine Aussage vorliegt")
	}
}
