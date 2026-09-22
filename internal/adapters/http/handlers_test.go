package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

func TestHabitatType_ReturnsGermanNameAdditively(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22?lang=de", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Typology string `json:"typology"`
		Code     string `json:"code"`
		NameEN   string `json:"name_en"`
		NameDE   *struct {
			Value      string `json:"value"`
			Vernacular string `json:"vernacular"`
			Provenance string `json:"provenance"`
			Source     string `json:"source"`
		} `json:"name_de"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if got.NameEN == "" {
		t.Error("name_en missing — the English name stays the identity even with lang=de")
	}
	if got.NameDE == nil {
		t.Fatal("name_de missing, want the German overlay object")
	}
	// The point of the object: a client cannot read value without provenance
	// sitting next to it, so an authored translation cannot pass as official.
	if got.NameDE.Value == "" || got.NameDE.Provenance == "" || got.NameDE.Source == "" {
		t.Errorf("name_de = %+v, want value, provenance and source all populated", got.NameDE)
	}
	if got.Typology != "eunis@2021" || got.Code != "R22" {
		t.Errorf("typology/code = %q/%q, want the requested key", got.Typology, got.Code)
	}
}

func TestHabitatType_SyntaxonAuthorAndParentIDAreOmittedWhenEmpty(t *testing.T) {
	q := seededQueryService()
	detail := q.types["eunis@2021:R22"]
	detail.Syntaxa = []input.SyntaxonRef{
		{ID: "CAK-01C", Rank: "alliance", Name: "Cakilion edentulae Br.-Bl. 1931", Author: "Br.-Bl. 1931", ParentID: "AA01"},
		{ID: "XYZ-01", Rank: "alliance", Name: "Nomatchion nowhereii"}, // no FloraVeg match
	}
	q.types["eunis@2021:R22"] = detail
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Syntaxa []struct {
			ID       string `json:"id"`
			Author   string `json:"author"`
			ParentID string `json:"parent_id"`
		} `json:"syntaxa"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if len(got.Syntaxa) != 2 {
		t.Fatalf("syntaxa = %+v, want 2 entries", got.Syntaxa)
	}
	if got.Syntaxa[0].Author != "Br.-Bl. 1931" || got.Syntaxa[0].ParentID != "AA01" {
		t.Errorf("matched syntaxon = %+v, want Author=%q ParentID=%q", got.Syntaxa[0], "Br.-Bl. 1931", "AA01")
	}

	// Decode into raw objects to check field ABSENCE, not just an empty string —
	// a substring match on the body would be brittle to field-ordering changes
	// in SyntaxonRef or a future encoder change.
	var raw struct {
		Syntaxa []map[string]any `json:"syntaxa"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decoding body as raw objects: %v", err)
	}
	if len(raw.Syntaxa) != 2 {
		t.Fatalf("syntaxa = %+v, want 2 entries", raw.Syntaxa)
	}
	unmatched := raw.Syntaxa[1]
	if _, ok := unmatched["author"]; ok {
		t.Errorf("unmatched syntaxon = %+v, want no \"author\" key", unmatched)
	}
	if _, ok := unmatched["parent_id"]; ok {
		t.Errorf("unmatched syntaxon = %+v, want no \"parent_id\" key", unmatched)
	}
}

func TestHabitatType_SyntaxonCarriesProvenanceFields(t *testing.T) {
	q := seededQueryService()
	detail := q.types["eunis@2021:R22"]
	detail.Syntaxa = []input.SyntaxonRef{
		{ID: "CAK-01C", Rank: "alliance", Name: "Cakilion edentulae Br.-Bl. 1931",
			EEACode: "TST-01A", Source: "evc", ParentProvenance: "official"},
	}
	q.types["eunis@2021:R22"] = detail
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Syntaxa []map[string]any `json:"syntaxa"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if len(got.Syntaxa) != 1 {
		t.Fatalf("syntaxa = %+v, want 1 entry", got.Syntaxa)
	}
	syn := got.Syntaxa[0]
	for field, want := range map[string]any{
		"eea_code":          "TST-01A",
		"source":            "evc",
		"parent_provenance": "official",
	} {
		if got := syn[field]; got != want {
			t.Errorf("%s = %v, erwartet %v", field, got, want)
		}
	}
}

func TestHabitatType_SyntaxonOmitsEmptyProvenanceFields(t *testing.T) {
	q := seededQueryService()
	detail := q.types["eunis@2021:R22"]
	detail.Syntaxa = []input.SyntaxonRef{
		{ID: "XYZ-01", Rank: "alliance", Name: "Nomatchion nowhereii"},
	}
	q.types["eunis@2021:R22"] = detail
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Syntaxa []map[string]any `json:"syntaxa"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if len(got.Syntaxa) != 1 {
		t.Fatalf("syntaxa = %+v, want 1 entry", got.Syntaxa)
	}
	syn := got.Syntaxa[0]
	for _, field := range []string{"eea_code", "source", "parent_provenance", "life_form_group"} {
		if _, ok := syn[field]; ok {
			t.Errorf("%s erscheint, obwohl leer", field)
		}
	}
}

func TestHabitatType_CrosswalksAreEmptyNotNullWhenNoneExist(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R99", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a type without any annex I match is normal)", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"crosswalks":[]`)) {
		t.Errorf("body = %s, want an empty crosswalks array", rec.Body)
	}
}

func TestHabitatType_UnknownTypologyIsInvalidQuery(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/bogus@1/R22", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
	}
}

// An unparseable typology never reaches the use case: the adapter rejects it.
func TestHabitatType_UnparseableTypologyIsInvalidQuery(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@/R22", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if q.habitatTypeCalls != 0 {
		t.Errorf("QueryService.HabitatType was called %d times, want 0 — parsing rejects it first", q.habitatTypeCalls)
	}
}

// A caller that names no typology gets the current EUNIS fassung.
func TestHabitatType_OmittedTypologyDefaultsToEunis2021(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/habitat-type/%20/R22", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — an omitted typology is not an error", rec.Code)
	}
	var got input.HabitatTypeSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	if got.Typology != domain.DefaultTypologyID {
		t.Errorf("typology = %q, want the default %q", got.Typology, domain.DefaultTypologyID)
	}
}

// A code that does not exist inside a known typology is a missing answer, not a
// malformed question.
func TestHabitatType_UnknownCodeIsNotFound(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/NOPE", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"NOT_FOUND"`)) {
		t.Errorf("body = %s, want the NOT_FOUND error envelope", rec.Body)
	}
}

// Annex I is addressed through the very same route — that is the whole point of
// keying types by (typology, code).
func TestHabitatType_AnnexIUsesTheSameRoute(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/habitat-type/annex1/6510", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got input.HabitatTypeDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	if got.Typology != "annex1" {
		t.Errorf("typology = %q, want annex1", got.Typology)
	}
	if len(got.Crosswalks) == 0 {
		t.Error("crosswalks are empty — the Annex I direction must see its EUNIS counterparts")
	}
}

// The three role buckets are always present so a client never has to guess
// whether an absent key means "no data" or "no such role".
func TestHabitatType_SpeciesCarriesEveryRoleBucket(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R99", nil))

	var got input.HabitatTypeDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	for _, role := range []string{input.RoleDiagnostic, input.RoleConstant, input.RoleDominant} {
		if _, ok := got.Species[role]; !ok {
			t.Errorf("species[%q] missing, want the bucket present (empty is fine)", role)
		}
	}
}

// provenance/derived_from must appear only on a derived entry — the common
// observed shape stays byte-identical to before, for existing clients.
func TestHabitatTypeSpecies_ProvenanceAndDerivedFromAreOmittedForObservedRows(t *testing.T) {
	q := seededQueryService()
	detail := q.types["eunis@2021:R22"]
	detail.Species[input.RoleDiagnostic] = []input.SpeciesEntry{
		{ConceptID: "wcvp-1", VerbatimName: "Bromus erectus", Role: input.RoleDiagnostic},
		{
			ConceptID: "wcvp-2", VerbatimName: "Rubus caesius", Role: input.RoleDiagnostic,
			Provenance:  "derived_from_aggregate",
			DerivedFrom: &input.AggregateSource{ConceptID: "wcvp-99", Name: "Rubus fruticosus aggr."},
		},
	}
	q.types["eunis@2021:R22"] = detail
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"concept_id":"wcvp-2","verbatim_name":"Rubus caesius","role":"diagnostic","provenance":"derived_from_aggregate","derived_from":{"concept_id":"wcvp-99","name":"Rubus fruticosus aggr."}`) {
		t.Errorf("body = %s, want the derived entry's provenance/derived_from present in this exact shape", body)
	}
	var got struct {
		Species map[string][]struct {
			VerbatimName string `json:"verbatim_name"`
			Provenance   string `json:"provenance"`
		} `json:"species"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	for _, e := range got.Species[input.RoleDiagnostic] {
		if e.VerbatimName == "Bromus erectus" && e.Provenance != "" {
			t.Errorf("observed entry Provenance = %q, want empty (omitted)", e.Provenance)
		}
	}
}

func TestHabitatTypeSpecies_FiltersByRole(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/v1/habitat-type/eunis@2021/R22/species?role=diagnostic", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.speciesRoleFilter != input.RoleDiagnostic {
		t.Errorf("role filter reached the use case as %q, want %q", q.speciesRoleFilter, input.RoleDiagnostic)
	}
	var got []input.SpeciesEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	if len(got) == 0 {
		t.Fatalf("body = %s, want the diagnostic species", rec.Body)
	}
	for _, e := range got {
		if e.Role != input.RoleDiagnostic {
			t.Errorf("role = %q, want only diagnostic entries", e.Role)
		}
	}
}

func TestHabitatTypeSpecies_UnknownRoleIsInvalidQuery(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/v1/habitat-type/eunis@2021/R22/species?role=beliebig", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
	}
}

// A regression that drops the filter before it reaches the use case (or never
// reads only_in_area) would leave the whole suite green otherwise — none of
// the other tests inspect what the use case actually received.
func TestHabitatTypeSpecies_AreaFilterReachesTheUseCase(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/v1/habitat-type/eunis@2021/R22/species?area=GER&only_in_area=true", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := input.AreaFilter{Code: "GER", OnlyInArea: true}
	if q.areaFilter != want {
		t.Errorf("filter reached the use case as %+v, want %+v", q.areaFilter, want)
	}
}

// only_in_area is a boolean the client sets explicitly, and a value that does
// not parse as one must not be silently read as false — the same rule as an
// unknown area code.
func TestHabitatTypeSpecies_UnparseableOnlyInAreaIsInvalidQuery(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/v1/habitat-type/eunis@2021/R22/species?only_in_area=beliebig", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
	}
}

// only_in_area=true without an area is a plausible caller (a client that
// always sends the flag, only sometimes with a fix on the area), not an error.
func TestHabitatTypeSpecies_OnlyInAreaWithoutAreaIsNotRejected(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/v1/habitat-type/eunis@2021/R22/species?only_in_area=true", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — only_in_area without area is a no-op, not an error", rec.Code)
	}
	if q.areaFilter.Active() {
		t.Errorf("filter = %+v, want an inactive filter (no area code was given)", q.areaFilter)
	}
}

func TestSpeciesHabitatTypes_ByConceptID(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/wcvp-1/habitat-types", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []input.HabitatTypeRole
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d habitat types, want 1", len(got))
	}
	if got[0].Role != input.RoleDiagnostic || got[0].Code != "R22" {
		t.Errorf("got %+v, want R22 as diagnostic", got[0])
	}
	if len(got[0].Syntaxa) == 0 {
		t.Error("syntaxa are empty — the species answer carries the type's syntaxa")
	}
}

// Accept-Language is honored just like ?lang=, and the query parameter wins.
func TestSpeciesHabitatTypes_LanguageFromAcceptLanguageHeader(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	req := httptest.NewRequest(http.MethodGet, "/v1/species/wcvp-1/habitat-types", nil)
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9,en;q=0.8")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.lang != "de" {
		t.Errorf("lang reached the use case as %q, want de", q.lang)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"name_en"`)) {
		t.Errorf("body = %s, want name_en present even for a German request", rec.Body)
	}
}

// An unsupported language is not an error: the answer falls back to English
// rather than rejecting a browser's Accept-Language.
func TestSpeciesHabitatTypes_UnsupportedLanguageFallsBackToEnglish(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	req := httptest.NewRequest(http.MethodGet, "/v1/species/wcvp-1/habitat-types", nil)
	req.Header.Set("Accept-Language", "fr-FR")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.lang != "en" {
		t.Errorf("lang reached the use case as %q, want the en fallback", q.lang)
	}
}

// An explicit ?lang= the service cannot serve must not silently fall through to
// Accept-Language: the caller asked for neither German nor English, so it gets
// the documented default, not the browser's preference.
func TestSpeciesHabitatTypes_ExplicitUnsupportedLanguageDoesNotFallThroughToTheHeader(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	req := httptest.NewRequest(http.MethodGet, "/v1/species/wcvp-1/habitat-types?lang=fr", nil)
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.lang != "en" {
		t.Errorf("lang reached the use case as %q, want the en fallback — ?lang= wins over the header", q.lang)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte(`"name_de"`)) {
		t.Errorf("body = %s, want no German overlay for a caller that asked for French", rec.Body)
	}
}

func TestSpeciesHabitatTypes_UnknownConceptIsNotFound(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/wcvp-nope/habitat-types", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"NOT_FOUND"`)) {
		t.Errorf("body = %s, want the NOT_FOUND error envelope", rec.Body)
	}
}

// A blank path segment is reachable (%20) and is a malformed question, not a
// missing answer.
func TestSpeciesHabitatTypes_BlankConceptIDIsInvalidQuery(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/species/%20/habitat-types", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
	}
}

func TestSyntaxonHabitatTypes_ReturnsTheLinkedTypes(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxon/BRO-01A/habitat-types", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []input.HabitatTypeSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	if len(got) != 1 || got[0].Code != "R22" {
		t.Errorf("got %+v, want the single linked type R22", got)
	}
}

func TestSyntaxonHabitatTypes_UnknownSyntaxonIsNotFound(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/syntaxon/NOPE/habitat-types", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"NOT_FOUND"`)) {
		t.Errorf("body = %s, want the NOT_FOUND error envelope", rec.Body)
	}
}

// The batch is the excursion app's whole field record in one request: one entry
// per input concept id, in input order, duplicates included, so a client can
// pair response[i] with concept_ids[i] without knowing anything about the
// deduplication the use case does internally.
func TestSpeciesBatch_AnswersOneEntryPerConceptIDInOrder(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	body := `{"concept_ids":["wcvp:concept:1","cdm:concept:x","wcvp:concept:1"]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types", strings.NewReader(body))
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []struct {
		ConceptID string `json:"concept_id"`
		Known     bool   `json:"known"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3 (one per input, duplicate included)", len(got))
	}
	if got[1].Reason != "unknown_backbone" {
		t.Errorf("entry 1 reason = %q, want unknown_backbone", got[1].Reason)
	}
}

// The old verbatim-name body must be rejected, not quietly tolerated: after the
// autark rewrite there is no name resolution behind this route, so accepting
// `names` would answer an empty list to a caller that believes it asked
// something. DisallowUnknownFields is what makes this a 400.
func TestSpeciesBatch_RejectsTheOldNamesField(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types",
		strings.NewReader(`{"names":["Bromus erectus"]}`))
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — verbatim names are no longer accepted", rec.Code)
	}
}

// An unknown concept id is a normal 200 answer carrying known=false, never a
// 404 and never a dropped input — one unknown id must not fail a whole plot list.
func TestSpeciesBatch_ReportsUnknownConceptsWithoutFailingTheBatch(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1","wcvp:concept:nofacts"]}`))
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []input.ConceptResolution
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want one per input", len(got))
	}
	if !got[0].Known || len(got[0].HabitatTypes) == 0 {
		t.Errorf("got[0] = %+v, want the known concept with its habitat types", got[0])
	}
	if got[1].Known || got[1].Reason != input.ReasonUnknownConcept {
		t.Errorf("got[1] = %+v, want known=false with reason %q", got[1], input.ReasonUnknownConcept)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"habitat_types":[]`)) {
		t.Errorf("body = %s, want an empty habitat_types array for the unknown concept", rec.Body)
	}
}

// The area filter is a query parameter on a POST body route — easy to parse in
// the handler and then forget to pass on. Nothing else in this suite would notice.
func TestSpeciesBatch_AreaFilterReachesTheUseCase(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types?area=GER&only_in_area=true",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1"]}`))
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := input.AreaFilter{Code: "GER", OnlyInArea: true}
	if q.areaFilter != want {
		t.Errorf("filter reached the use case as %+v, want %+v", q.areaFilter, want)
	}
}

// A query parameter that does not parse must be rejected before the body is even
// read — and must not be silently read as false.
func TestSpeciesBatch_UnparseableOnlyInAreaIsInvalidQuery(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types?only_in_area=beliebig",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1"]}`))
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
	}
	if q.conceptSetCalls != 0 {
		t.Errorf("the use case was called %d times, want 0 for a rejected query string", q.conceptSetCalls)
	}
}

// The 1 MiB body cap does not bound the item count, and each distinct id costs
// index work — so the array length needs its own bound. The bound is on the raw
// array length, exactly as `maxItems` in the spec is defined, so a validating
// gateway and this handler cannot disagree: 301 entries are rejected even when
// they collapse to a single distinct id.
func TestSpeciesBatch_TooManyConceptIDsIsInvalidQuery(t *testing.T) {
	distinctList := make([]string, 0, 301)
	for i := range 301 {
		distinctList = append(distinctList, fmt.Sprintf("wcvp:concept:%d", i))
	}
	duplicateList := make([]string, 301)
	for i := range duplicateList {
		duplicateList[i] = "wcvp:concept:1"
	}

	for name, list := range map[string][]string{
		"301 distinct ids":               distinctList,
		"301 entries of one repeated id": duplicateList,
	} {
		t.Run(name, func(t *testing.T) {
			q := seededQueryService()
			srv := newTestServer(t, q)

			body, err := json.Marshal(map[string][]string{"concept_ids": list})
			if err != nil {
				t.Fatalf("encoding body: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			srv.Router().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
				t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
			}
			if q.conceptSetCalls != 0 {
				t.Errorf("the use case was called %d times, want no call at all", q.conceptSetCalls)
			}
		})
	}
}

// A body carrying more than one JSON value must be rejected whole. Decoding the
// first and discarding the rest would leave the caller believing it sent both.
func TestSpeciesBatch_TrailingDataIsInvalidQuery(t *testing.T) {
	for name, body := range map[string]string{
		"two objects":         `{"concept_ids":["wcvp:concept:1"]}{"concept_ids":["wcvp:concept:2"]}`,
		"object then garbage": `{"concept_ids":["wcvp:concept:1"]} nonsense`,
		"object then array":   `{"concept_ids":["wcvp:concept:1"]}[]`,
	} {
		t.Run(name, func(t *testing.T) {
			q := seededQueryService()
			srv := newTestServer(t, q)

			req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.Router().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
				t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
			}
			if q.conceptSetCalls != 0 {
				t.Errorf("the use case was called %d times, want no call for a rejected body", q.conceptSetCalls)
			}
		})
	}
}

// A blank entry must be rejected, not skipped. Skipping it would return fewer
// entries than were asked about, and the whole contract of this route is that
// response[i] belongs to concept_ids[i] — a client that trusts that and sends
// one stray empty string would shift its entire plot list by one. Reporting it
// as unknown_backbone would be a lie (an empty string is not another backbone),
// so it gets the same verdict a typo'd area code gets: INVALID_QUERY.
func TestSpeciesBatch_BlankConceptIDBetweenValidOnesIsInvalidQuery(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1","  ","wcvp:concept:2"]}`))
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — a dropped entry would break response[i] <-> concept_ids[i]", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
	}
	if q.conceptSetCalls != 0 {
		t.Errorf("the use case was called %d times, want no call for a rejected body", q.conceptSetCalls)
	}
}

func TestSpeciesBatch_MalformedBodyIsInvalidQuery(t *testing.T) {
	for name, body := range map[string]string{
		"not json":         `{`,
		"no concept ids":   `{"concept_ids":[]}`,
		"blank concept id": `{"concept_ids":["  "]}`,
	} {
		t.Run(name, func(t *testing.T) {
			q := seededQueryService()
			srv := newTestServer(t, q)

			req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.Router().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
				t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
			}
			// An empty array must be rejected, not answered with an empty list:
			// asking about nothing is a mistake in the caller, not a question.
			if q.conceptSetCalls != 0 {
				t.Errorf("the use case was called %d times, want no call for a rejected body", q.conceptSetCalls)
			}
		})
	}
}

// An unclassifiable failure from the use case on the batch route is a 500 with
// the envelope, never a partial answer.
func TestSpeciesBatch_UnexpectedFailureIsInternalError(t *testing.T) {
	q := seededQueryService()
	q.err = fmt.Errorf("index is on fire")
	srv := newTestServer(t, q)

	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types",
		strings.NewReader(`{"concept_ids":["wcvp:concept:1"]}`))
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INTERNAL_ERROR"`)) {
		t.Errorf("body = %s, want the INTERNAL_ERROR error envelope", rec.Body)
	}
}

// An unclassifiable failure from the use case is a 500 with the envelope, never
// a partial answer.
func TestHabitatType_UnexpectedFailureIsInternalError(t *testing.T) {
	q := seededQueryService()
	q.err = fmt.Errorf("index is on fire")
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INTERNAL_ERROR"`)) {
		t.Errorf("body = %s, want the INTERNAL_ERROR error envelope", rec.Body)
	}
}

// unknownAreaQueryService doubles a use case that rejected an ?area= the index
// has no data for.
func unknownAreaQueryService() *fakeQueryService {
	q := seededQueryService()
	q.err = fmt.Errorf("area %q: %w", "NOPE", input.ErrUnknownArea)
	return q
}

// --- doubles ------------------------------------------------------------------

// fakeQueryService is a seeded double for input.QueryService. The HTTP layer is
// tested against the port, never against sqlite: what matters here is parsing,
// status mapping and serialization. It records the lang and role it was called
// with so the tests can prove those reached the use case.
type fakeQueryService struct {
	types             map[string]input.HabitatTypeDetail
	byConcept         map[string][]input.HabitatTypeRole
	bySyntaxon        map[string][]input.HabitatTypeSummary
	err               error
	lang              string
	speciesRoleFilter string
	habitatTypeCalls  int
	// conceptSetCalls counts the batch use case, so a test can prove a rejected
	// request never reached it.
	conceptSetCalls int
	// areaFilter records what the last call of any of the three area-aware
	// methods received, so a test can assert the parsed query string actually
	// reached the use case, not just that the route parses.
	areaFilter input.AreaFilter
	// indexInfo is what /v1/info reports; indexInfoErr fails it, so the
	// handler's failure path is exercised without a broken index.
	indexInfo    input.IndexInfo
	indexInfoErr error
	// traitSets/traitErr control what Traits returns; gotTraitConceptID and
	// gotTraitVocab record its last call's arguments, analogous to
	// speciesRoleFilter/areaFilter above.
	traitSets         []input.TraitSetView
	traitErr          error
	gotTraitConceptID string
	gotTraitVocab     string
	// searchHits/searchErr control what SearchSpecies returns.
	searchHits []input.SpeciesSearchHit
	searchErr  error
	// traitSummaries/traitSummaryErr control what SpeciesTraitSummary
	// returns, keyed by concept id.
	traitSummaries  map[string]map[string]input.VocabSummary
	traitSummaryErr error
	// typologies/typologiesErr control what Typologies returns.
	typologies    []input.TypologyView
	typologiesErr error
	// areas/areasErr control what Areas returns; areaScheme records the last
	// scheme it was called with, so a test can prove the parsed query string
	// (or its default) actually reached the use case.
	areas      []input.AreaView
	areasErr   error
	areaScheme string
	// undescribed strips the description fields, standing for the 7673 types
	// the factsheets do not cover.
	undescribed bool
	// syntaxonDetails backs GET /v1/syntaxon/{id}; syntaxaByRank backs
	// GET /v1/syntaxa, keyed by "rank|life_form_group" so a test can pin the
	// handler's default and its group pass-through separately.
	syntaxonDetails map[string]input.SyntaxonDetail
	syntaxaByRank   map[string][]input.SyntaxonRef
	syntaxaErr      error
	// gotRank/gotLifeFormGroup record the last SyntaxaByRank call; rankCalls
	// counts it, so a test can prove a rejected filter never reached the port.
	gotRank          string
	gotLifeFormGroup string
	rankCalls        int
	// syntaxonAreaFilter records the last filter SyntaxaByRank received, so a
	// test can prove the parsed ?area=/?include= actually reached the port.
	syntaxonAreaFilter input.SyntaxonAreaFilter
}

func (f *fakeQueryService) Areas(_ context.Context, scheme string) ([]input.AreaView, error) {
	f.areaScheme = scheme
	if f.areasErr != nil {
		return nil, f.areasErr
	}
	return f.areas, nil
}

func (f *fakeQueryService) Typologies(context.Context) ([]input.TypologyView, error) {
	if f.typologiesErr != nil {
		return nil, f.typologiesErr
	}
	return f.typologies, nil
}

// SpeciesTraitSummary mirrors the use case's contract closely enough for the
// adapter to be tested against it: known concepts report the seeded
// vocabularies, unknown ones report unknown_concept. Deliberately simple —
// callers of this double keep the request to a single known concept id, since
// this fake overwrites its own Vocabularies assignment on a second known id
// rather than merging.
func (f *fakeQueryService) SpeciesTraitSummary(_ context.Context, conceptIDs []string) (input.TraitSummary, error) {
	if f.traitSummaryErr != nil {
		return input.TraitSummary{}, f.traitSummaryErr
	}
	if len(conceptIDs) == 0 {
		return input.TraitSummary{}, fmt.Errorf("concept_ids must hold at least one id: %w", input.ErrInvalidQuery)
	}
	summary := input.TraitSummary{
		Requested:    len(conceptIDs),
		Unknown:      []input.UnknownConcept{},
		Vocabularies: map[string]input.VocabSummary{},
	}
	for _, id := range conceptIDs {
		if values, ok := f.traitSummaries[id]; ok {
			summary.Known++
			summary.Vocabularies = values
			continue
		}
		summary.Unknown = append(summary.Unknown,
			input.UnknownConcept{ConceptID: id, Reason: input.ReasonUnknownConcept})
	}
	return summary, nil
}

func (f *fakeQueryService) SearchSpecies(_ context.Context, q string, limit *int) ([]input.SpeciesSearchHit, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	// Mirrors the real use case's contract, not its implementation: an
	// empty/whitespace-only q and a non-nil limit outside [1, MaxSearchLimit]
	// — an explicit 0 included — are rejected with input.ErrInvalidQuery;
	// limit == nil means "unset" and is not one of those rejected values.
	if strings.TrimSpace(q) == "" {
		return nil, fmt.Errorf("q must not be empty: %w", input.ErrInvalidQuery)
	}
	if limit != nil && (*limit <= 0 || *limit > input.MaxSearchLimit) {
		return nil, fmt.Errorf("limit must be between 1 and %d: %w", input.MaxSearchLimit, input.ErrInvalidQuery)
	}
	out := []input.SpeciesSearchHit{}
	for _, h := range f.searchHits {
		if strings.Contains(strings.ToLower(h.VerbatimName), strings.ToLower(q)) {
			out = append(out, h)
		}
		if limit != nil && len(out) == *limit {
			break
		}
	}
	return out, nil
}

func (f *fakeQueryService) Traits(_ context.Context, conceptID, vocab string) ([]input.TraitSetView, error) {
	f.gotTraitConceptID, f.gotTraitVocab = conceptID, vocab
	if f.traitErr != nil {
		return nil, f.traitErr
	}
	return f.traitSets, nil
}

func (f *fakeQueryService) IndexInfo(context.Context) (input.IndexInfo, error) {
	if f.indexInfoErr != nil {
		return input.IndexInfo{}, f.indexInfoErr
	}
	return f.indexInfo, nil
}

func (f *fakeQueryService) Syntaxon(_ context.Context, id, lang string) (input.SyntaxonDetail, error) {
	f.lang = lang
	if f.err != nil {
		return input.SyntaxonDetail{}, f.err
	}
	detail, ok := f.syntaxonDetails[id]
	if !ok {
		return input.SyntaxonDetail{}, fmt.Errorf("syntaxon %q: %w", id, input.ErrNotFound)
	}
	return detail, nil
}

// SyntaxaByRank restates the use case's contract closely enough for the adapter
// to be tested against it: an unknown rank is INVALID_QUERY with the allowed
// values in the message, never an empty list.
func (f *fakeQueryService) SyntaxaByRank(_ context.Context, rank, lifeFormGroup string,
	filter input.SyntaxonAreaFilter) ([]input.SyntaxonRef, error) {
	f.rankCalls++
	f.gotRank, f.gotLifeFormGroup = rank, lifeFormGroup
	f.syntaxonAreaFilter = filter
	if f.syntaxaErr != nil {
		return nil, f.syntaxaErr
	}
	refs, ok := f.syntaxaByRank[rank+"|"+lifeFormGroup]
	if !ok {
		return nil, fmt.Errorf("rank %q: the index carries alliance, class, formation, order: %w",
			rank, input.ErrInvalidQuery)
	}
	return refs, nil
}

func seededQueryService() *fakeQueryService {
	level := 3
	priority := false
	syntaxa := []input.SyntaxonRef{{ID: "BRO-01A", Rank: "alliance", Name: "Bromion erecti"}}
	r22 := input.HabitatTypeSummary{
		Typology: "eunis@2021", Code: "R22", Level: &level,
		NameEN: "Low and medium altitude hay meadow",
	}
	return &fakeQueryService{
		types: map[string]input.HabitatTypeDetail{
			"eunis@2021:R22": {
				HabitatTypeSummary: r22,
				Description: &input.DescriptionText{
					Value:      "Hay meadows of lowland and montane areas.",
					Provenance: "official", Source: "floraveg:2021-06-01",
				},
				DescriptionDE: &input.GermanLabel{
					Value:      "Mähwiesen der Tief- und mittleren Lagen.",
					Provenance: "situs", Source: "situs@0.7.0",
				},
				Species: map[string][]input.SpeciesEntry{
					input.RoleDiagnostic: {{ConceptID: "wcvp-1", VerbatimName: "Bromus erectus", Role: input.RoleDiagnostic}},
					input.RoleConstant:   {},
					input.RoleDominant:   {},
				},
				Syntaxa:    syntaxa,
				Crosswalks: []input.CrosswalkRef{{Typology: "annex1", Code: "6510", Qualifier: domain.QualifierSame}},
			},
			"eunis@2021:R99": {
				HabitatTypeSummary: input.HabitatTypeSummary{Typology: "eunis@2021", Code: "R99", NameEN: "Lonely type"},
				Species: map[string][]input.SpeciesEntry{
					input.RoleDiagnostic: {}, input.RoleConstant: {}, input.RoleDominant: {},
				},
				Syntaxa:    []input.SyntaxonRef{},
				Crosswalks: []input.CrosswalkRef{},
			},
			"annex1:6510": {
				HabitatTypeSummary: input.HabitatTypeSummary{
					Typology: "annex1", Code: "6510", NameEN: "Lowland hay meadows", Priority: &priority,
				},
				Species: map[string][]input.SpeciesEntry{
					input.RoleDiagnostic: {}, input.RoleConstant: {}, input.RoleDominant: {},
				},
				Syntaxa:    []input.SyntaxonRef{},
				Crosswalks: []input.CrosswalkRef{{Typology: "eunis@2021", Code: "R22", Qualifier: domain.QualifierSame}},
			},
		},
		byConcept: map[string][]input.HabitatTypeRole{
			"wcvp-1":         {{HabitatTypeSummary: r22, Role: input.RoleDiagnostic, Syntaxa: syntaxa}},
			"wcvp:concept:1": {{HabitatTypeSummary: r22, Role: input.RoleDiagnostic, Syntaxa: syntaxa}},
		},
		bySyntaxon: map[string][]input.HabitatTypeSummary{"BRO-01A": {r22}},
		areas: []input.AreaView{
			{Scheme: domain.SchemeWGSRPDL3, Code: "FRA", Name: "France"},
			{Scheme: domain.SchemeWGSRPDL3, Code: "GER", Name: "Germany"},
		},
		typologies: []input.TypologyView{
			{ID: "annex1", Scheme: "annex1", Version: "92/43/EEC", Name: "Habitats Directive Annex I", HabitatTypes: 1},
			{ID: "eunis@2021", Scheme: "eunis", Version: "2021", Name: "EUNIS 2021", HabitatTypes: 2},
		},
		indexInfo: input.IndexInfo{
			ConceptBackbones:        []string{"wcvp"},
			SpeciesWithConcept:      2,
			AreaScheme:              domain.SchemeWGSRPDL3,
			AreasWithData:           3,
			SyntaxonAreaScheme:      domain.SchemeEVCTerritory,
			SyntaxaWithDistribution: 1,
		},
		searchHits: []input.SpeciesSearchHit{
			{VerbatimName: "Fagus sylvatica", ConceptID: strPtr("wcvp:concept:83891")},
			{VerbatimName: "Fagus orientalis"},
		},
		traitSummaries: map[string]map[string]input.VocabSummary{
			"wcvp:concept:83891": {
				"eive": {VocabVersion: "1.0", Dimensions: map[string]input.DimensionSummary{
					"M": {Mean: 6, Min: 4, Max: 8, N: 2},
				}},
			},
		},
		syntaxonDetails: map[string]input.SyntaxonDetail{
			"C": {
				SyntaxonRef: input.SyntaxonRef{ID: "C", Rank: "formation",
					Name: "Vegetation of the nemoral forest zone", LifeFormGroup: "phanerogam"},
				Ancestors: []input.SyntaxonRef{},
				Children: []input.SyntaxonRef{
					{ID: "CA", Rank: "class", Name: "Testklasse", ParentID: "C"},
				},
			},
			"BRO-01A": {
				SyntaxonRef: input.SyntaxonRef{ID: "BRO-01A", Rank: "alliance",
					Name: "Bromion erecti", Author: "Koch 1926", ParentID: "CA01",
					EEACode: "BRO-01A", Source: "evc", ParentProvenance: "official",
					LifeFormGroup: "phanerogam"},
				Ancestors: []input.SyntaxonRef{
					{ID: "C", Rank: "formation", Name: "Vegetation of the nemoral forest zone"},
					{ID: "CA", Rank: "class", Name: "Testklasse", ParentID: "C"},
					{ID: "CA01", Rank: "order", Name: "Testordnung", ParentID: "CA"},
				},
				Children:               []input.SyntaxonRef{},
				DirectHabitatTypeCount: 1,
			},
			// CA01B: the source checked and found no occurrence at all — the
			// present-object-with-two-empty-lists case.
			"CA01B": {
				SyntaxonRef: input.SyntaxonRef{ID: "CA01B", Rank: "alliance", Name: "Testverband ohne Vorkommen"},
				Ancestors:   []input.SyntaxonRef{},
				Children:    []input.SyntaxonRef{},
				Distribution: &input.SyntaxonDistribution{
					AreaScheme: domain.SchemeEVCTerritory,
					Verified:   []string{},
					Uncertain:  []string{},
				},
			},
			// RA01A: a bryophyte alliance the source makes no statement about
			// at all — Distribution stays nil, and the JSON field is absent.
			"RA01A": {
				SyntaxonRef: input.SyntaxonRef{ID: "RA01A", Rank: "alliance", Name: "Testverband ohne Abdeckung"},
				Ancestors:   []input.SyntaxonRef{},
				Children:    []input.SyntaxonRef{},
			},
		},
		syntaxaByRank: map[string][]input.SyntaxonRef{
			"formation|": {{ID: "C", Rank: "formation",
				Name: "Vegetation of the nemoral forest zone", LifeFormGroup: "phanerogam"}},
			"formation|bryophyte_lichen": {{ID: "R", Rank: "formation",
				Name: "Epigaeic bryophyte and lichen vegetation", LifeFormGroup: "bryophyte_lichen"}},
			"class|": {{ID: "CA", Rank: "class", Name: "Testklasse", ParentID: "C"}},
			// alliance| carries the ?area= JSON-shape fixture: CA01A a
			// verified hit, RA01A a carried-but-unjudged row. The fake
			// returns this canned answer regardless of the actual filter
			// value — it exists to pin the wire shape, not the filter logic
			// (which internal/application/syntaxon_distribution_test.go
			// already covers).
			"alliance|": {
				{ID: "CA01A", Rank: "alliance", Name: "Verband mit Verbreitung",
					Occurrence: domain.OccurrenceVerified},
				{ID: "RA01A", Rank: "alliance", Name: "Verband ohne Aussage"},
			},
		},
	}
}

// germanOverlay mimics what the real use case does for lang=de: it adds the
// German label, it never replaces name_en.
func germanOverlay(s input.HabitatTypeSummary, lang string) input.HabitatTypeSummary {
	if lang == "de" {
		s.NameDE = &input.GermanLabel{
			Value:      "Magere Flachland-Mähwiese",
			Vernacular: "Glatthaferwiese",
			Provenance: "derived",
			Source:     "derived-annex1",
		}
	}
	return s
}

func (f *fakeQueryService) HabitatType(_ context.Context, key domain.HabitatTypeKey, lang string, filter input.AreaFilter) (input.HabitatTypeDetail, error) {
	f.habitatTypeCalls++
	f.lang = lang
	f.areaFilter = filter
	if f.err != nil {
		return input.HabitatTypeDetail{}, f.err
	}
	if key.Typology != "eunis@2021" && key.Typology != "annex1" {
		return input.HabitatTypeDetail{}, fmt.Errorf("typology %s: %w", key.Typology, input.ErrUnknownTypology)
	}
	detail, ok := f.types[key.String()]
	if !ok {
		return input.HabitatTypeDetail{}, fmt.Errorf("habitat type %s: %w", key, input.ErrNotFound)
	}
	detail.HabitatTypeSummary = germanOverlay(detail.HabitatTypeSummary, lang)
	if f.undescribed {
		detail.Description, detail.DescriptionDE = nil, nil
	} else if lang != "de" {
		detail.DescriptionDE = nil
	}
	return detail, nil
}

func (f *fakeQueryService) SpeciesHabitatTypes(_ context.Context, conceptID, lang string, filter input.AreaFilter) ([]input.HabitatTypeRole, error) {
	f.lang = lang
	f.areaFilter = filter
	if f.err != nil {
		return nil, f.err
	}
	roles, ok := f.byConcept[conceptID]
	if !ok {
		return nil, fmt.Errorf("concept %s: %w", conceptID, input.ErrNotFound)
	}
	out := make([]input.HabitatTypeRole, 0, len(roles))
	for _, r := range roles {
		r.HabitatTypeSummary = germanOverlay(r.HabitatTypeSummary, lang)
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeQueryService) HabitatTypeSpecies(_ context.Context, key domain.HabitatTypeKey, role string, filter input.AreaFilter) ([]input.SpeciesEntry, error) {
	f.speciesRoleFilter = role
	f.areaFilter = filter
	if f.err != nil {
		return nil, f.err
	}
	detail, ok := f.types[key.String()]
	if !ok {
		return nil, fmt.Errorf("habitat type %s: %w", key, input.ErrNotFound)
	}
	out := []input.SpeciesEntry{}
	for r, entries := range detail.Species {
		if role != "" && r != role {
			continue
		}
		out = append(out, entries...)
	}
	return out, nil
}

func (f *fakeQueryService) SyntaxonHabitatTypes(_ context.Context, syntaxonID, lang string) ([]input.HabitatTypeSummary, error) {
	f.lang = lang
	if f.err != nil {
		return nil, f.err
	}
	types, ok := f.bySyntaxon[syntaxonID]
	if !ok {
		return nil, fmt.Errorf("syntaxon %s: %w", syntaxonID, input.ErrNotFound)
	}
	return types, nil
}

// SpeciesSetHabitatTypes mirrors the use case's contract closely enough for the
// adapter to be tested against it: one entry per input in input order, and the
// two reasons kept apart. It is deliberately a re-statement of the contract, not
// a call into the real service — the HTTP layer is tested against the port.
func (f *fakeQueryService) SpeciesSetHabitatTypes(ctx context.Context, conceptIDs []string, lang string, filter input.AreaFilter) ([]input.ConceptResolution, error) {
	f.conceptSetCalls++
	f.lang = lang
	// Recorded on the way out: the nested SpeciesHabitatTypes calls below record
	// their own (empty) filter and would otherwise overwrite this one.
	defer func() { f.areaFilter = filter }()
	if f.err != nil {
		return nil, f.err
	}
	out := make([]input.ConceptResolution, 0, len(conceptIDs))
	for _, id := range conceptIDs {
		entry := input.ConceptResolution{ConceptID: id, HabitatTypes: []input.HabitatTypeRole{}}
		switch {
		case !strings.HasPrefix(id, "wcvp:"):
			entry.Reason = input.ReasonUnknownBackbone
		default:
			types, err := f.SpeciesHabitatTypes(ctx, id, lang, input.AreaFilter{})
			if err != nil {
				entry.Reason = input.ReasonUnknownConcept
			} else {
				entry.Known, entry.HabitatTypes = true, types
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// Every request made before the second scheme existed must keep its answer
// unchanged. The default is not cosmetic: it is the compatibility promise.
func TestAreasOhneSchemaAntwortetWieBisher(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/areas", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.areaScheme != domain.SchemeWGSRPDL3 {
		t.Errorf("scheme = %q, want the default %q", q.areaScheme, domain.SchemeWGSRPDL3)
	}
}

func TestAreasMitTerritoriumsschema(t *testing.T) {
	q := seededQueryService()
	srv := newTestServer(t, q)

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/areas?scheme=evc_territory", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if q.areaScheme != domain.SchemeEVCTerritory {
		t.Errorf("scheme = %q, want %q", q.areaScheme, domain.SchemeEVCTerritory)
	}
}

func TestAreasMitUnbekanntemSchemaIstInvalidQuery(t *testing.T) {
	for _, scheme := range []string{"evc-territory", "iso3166", "WGSRPD_L3"} {
		srv := newTestServer(t, seededQueryService())

		rec := httptest.NewRecorder()
		srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/areas?scheme="+scheme, nil))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("scheme=%q: status = %d, want 400", scheme, rec.Code)
		}
		body := rec.Body.String()
		// The message lists the allowed values — never an empty list, which
		// would look like "there are none" while meaning "you mistyped".
		for _, want := range []string{"INVALID_QUERY", "evc_territory", "wgsrpd_l3"} {
			if !strings.Contains(body, want) {
				t.Errorf("scheme=%q: response does not mention %q: %s", scheme, want, body)
			}
		}
	}
}

func TestHabitatTypeSpecies_UnknownAreaIsInvalidQuery(t *testing.T) {
	srv := newTestServer(t, unknownAreaQueryService())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/habitat-type/eunis@2021/R22/species?area=NOPE", nil)
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY envelope", rec.Body)
	}
}

// An unknown area code on the species route is INVALID_QUERY even when the
// concept is unknown too: the malformed question is reported, not the missing
// answer. The ordering itself is pinned in the use case
// (TestSpeciesHabitatTypes_UnknownAreaOutranksAnUnknownConcept); this pins that
// the verdict reaches the wire as a 400 rather than a 404.
func TestSpeciesHabitatTypes_UnknownAreaIsInvalidQueryNotNotFound(t *testing.T) {
	srv := newTestServer(t, unknownAreaQueryService())

	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/v1/species/nope/habitat-types?area=NOPE", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — a typo'd area code outranks a missing concept", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
		t.Errorf("body = %s, want the INVALID_QUERY envelope", rec.Body)
	}
}
