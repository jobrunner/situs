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
		Typology         string `json:"typology"`
		Code             string `json:"code"`
		NameEN           string `json:"name_en"`
		NameDE           string `json:"name_de"`
		NameDEProvenance string `json:"name_de_provenance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if got.NameEN == "" {
		t.Error("name_en missing — the English name stays the identity even with lang=de")
	}
	if got.NameDE == "" || got.NameDEProvenance == "" {
		t.Errorf("name_de/%s missing, want the German overlay with its provenance", "name_de_provenance")
	}
	if got.Typology != "eunis@2021" || got.Code != "R22" {
		t.Errorf("typology/code = %q/%q, want the requested key", got.Typology, got.Code)
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
			srv := newTestServer(t, seededQueryService())

			req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.Router().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
				t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
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
}

func (f *fakeQueryService) IndexInfo(context.Context) (input.IndexInfo, error) {
	if f.indexInfoErr != nil {
		return input.IndexInfo{}, f.indexInfoErr
	}
	return f.indexInfo, nil
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
		indexInfo: input.IndexInfo{
			ConceptBackbones:   []string{"wcvp"},
			SpeciesWithConcept: 2,
			AreaScheme:         domain.SchemeWGSRPDL3,
			AreasWithData:      3,
		},
	}
}

// germanOverlay mimics what the real use case does for lang=de: it adds the
// German label, it never replaces name_en.
func germanOverlay(s input.HabitatTypeSummary, lang string) input.HabitatTypeSummary {
	if lang == "de" {
		s.NameDE = "Magere Flachland-Mähwiese"
		s.NameDEProvenance = "derived"
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
