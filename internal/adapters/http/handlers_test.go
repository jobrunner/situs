package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
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

func TestSpeciesBatch_ReportsResolvedAndUnresolvedNames(t *testing.T) {
	srv := newTestServer(t, seededQueryService())

	body := strings.NewReader(`{"names":["Bromus erectus","Nonexistens dubia"]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types", body)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []input.NameResolution
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want one per input name", len(got))
	}
	if !got[0].Resolved || got[0].ConceptID == "" || len(got[0].HabitatTypes) == 0 {
		t.Errorf("got[0] = %+v, want the resolved name with its habitat types", got[0])
	}
	if got[1].Resolved || got[1].Verbatim == "" {
		t.Errorf("got[1] = %+v, want the unresolved name kept with resolved=false", got[1])
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"habitat_types":[]`)) {
		t.Errorf("body = %s, want an empty habitat_types array for the unresolved name", rec.Body)
	}
}

// A field recording repeats names, and each 50 distinct names is one hostus
// round trip — so a duplicate must not buy a second one upstream. The answer,
// however, carries one entry per input name in input order, so a client can pair
// response[i] with names[i] without knowing anything about deduplication.
func TestSpeciesBatch_DeduplicatesUpstreamButAnswersPerInputName(t *testing.T) {
	names := seededNameQueryService()
	srv := newServerWithNames(t, seededQueryService(), names)

	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types",
		strings.NewReader(`{"names":["Salvia pratensis","Bromus erectus"," Salvia pratensis ","Bromus erectus"]}`))
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// Property one: each distinct name reached the resolver exactly once.
	if want := []string{"Salvia pratensis", "Bromus erectus"}; !slices.Equal(names.gotNames, want) {
		t.Errorf("resolver saw %q, want %q — deduplicated in input order", names.gotNames, want)
	}

	// Property two: one entry per input name, in input order, duplicates included.
	var got []input.NameResolution
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	verbatims := make([]string, 0, len(got))
	for _, r := range got {
		verbatims = append(verbatims, r.Verbatim)
	}
	want := []string{"Salvia pratensis", "Bromus erectus", "Salvia pratensis", "Bromus erectus"}
	if !slices.Equal(verbatims, want) {
		t.Fatalf("response verbatims = %q, want %q (one per input name, input order)", verbatims, want)
	}

	// A repeated name must carry the same resolution both times, not an empty one.
	if got[1].ConceptID != got[3].ConceptID || !got[1].Resolved || !got[3].Resolved {
		t.Errorf("duplicate entries = %+v / %+v, want the same resolution twice", got[1], got[3])
	}
	if len(got[1].HabitatTypes) == 0 || len(got[3].HabitatTypes) == 0 {
		t.Error("a duplicated resolved name lost its habitat types on one of its entries")
	}
}

// partialNameQueryService answers about fewer names than it was asked about —
// the one way the fan-out can find no resolution for an input name.
type partialNameQueryService struct{}

func (partialNameQueryService) SpeciesHabitatTypesByName(
	_ context.Context, names []string, _ string,
) ([]input.NameResolution, error) {
	return []input.NameResolution{
		{Verbatim: names[0], Resolved: true, ConceptID: "wcvp-1", HabitatTypes: []input.HabitatTypeRole{}},
	}, nil
}

// An input name the use case said nothing about must still come back — an input
// is never silently dropped, and habitat_types must be [] rather than null.
func TestSpeciesBatch_InputNameWithoutAResolutionIsStillReported(t *testing.T) {
	srv := newServerWithNames(t, seededQueryService(), partialNameQueryService{})

	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types",
		strings.NewReader(`{"names":["Bromus erectus","Salvia pratensis"]}`))
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []input.NameResolution
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %s: %v", rec.Body, err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want one per input name even so", len(got))
	}
	if got[1].Verbatim != "Salvia pratensis" || got[1].Resolved {
		t.Errorf("got[1] = %+v, want the unanswered name reported with resolved=false", got[1])
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"habitat_types":[]`)) {
		t.Errorf("body = %s, want habitat_types as [] rather than null", rec.Body)
	}
}

// The 1 MiB body cap does not bound the item count, and each 50 names is a
// hostus round trip — so the array length needs its own bound. The bound is on
// the raw array length, exactly as `maxItems` in the spec is defined, so a
// validating gateway and this handler cannot disagree: 301 entries are rejected
// even when they collapse to a single distinct name.
func TestSpeciesBatch_TooManyNamesIsInvalidQuery(t *testing.T) {
	distinctList := make([]string, 0, 301)
	for i := range 301 {
		distinctList = append(distinctList, fmt.Sprintf("Genus species%d", i))
	}
	duplicateList := make([]string, 301)
	for i := range duplicateList {
		duplicateList[i] = "Bromus erectus"
	}

	for name, list := range map[string][]string{
		"301 distinct names":               distinctList,
		"301 entries of one repeated name": duplicateList,
	} {
		t.Run(name, func(t *testing.T) {
			names := seededNameQueryService()
			srv := newServerWithNames(t, seededQueryService(), names)

			body, err := json.Marshal(map[string][]string{"names": list})
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
			if names.gotNames != nil {
				t.Errorf("resolver was called with %d names, want no upstream call at all", len(names.gotNames))
			}
		})
	}
}

// A body carrying more than one JSON value must be rejected whole. Decoding the
// first and discarding the rest would leave the caller believing it sent both.
func TestSpeciesBatch_TrailingDataIsInvalidQuery(t *testing.T) {
	for name, body := range map[string]string{
		"two objects":         `{"names":["Bromus erectus"]}{"names":["Inula hirta"]}`,
		"object then garbage": `{"names":["Bromus erectus"]} nonsense`,
		"object then array":   `{"names":["Bromus erectus"]}[]`,
	} {
		t.Run(name, func(t *testing.T) {
			names := seededNameQueryService()
			srv := newServerWithNames(t, seededQueryService(), names)

			req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.Router().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte(`"INVALID_QUERY"`)) {
				t.Errorf("body = %s, want the INVALID_QUERY error envelope", rec.Body)
			}
			if names.gotNames != nil {
				t.Errorf("resolver was called with %v, want no upstream call for a rejected body", names.gotNames)
			}
		})
	}
}

func TestSpeciesBatch_MalformedBodyIsInvalidQuery(t *testing.T) {
	for name, body := range map[string]string{
		"not json":   `{`,
		"no names":   `{"names":[]}`,
		"blank name": `{"names":["  "]}`,
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

// hostus being down must be visible as such, not as a 500 or an empty answer.
func TestSpeciesBatch_UpstreamFailureIsReportedAsSuch(t *testing.T) {
	names := seededNameQueryService()
	names.err = fmt.Errorf("dialing hostus: %w", input.ErrUpstreamUnavailable)
	srv := newServerWithNames(t, seededQueryService(), names)

	req := httptest.NewRequest(http.MethodPost, "/v1/species/habitat-types",
		strings.NewReader(`{"names":["Bromus erectus"]}`))
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"UPSTREAM_UNAVAILABLE"`)) {
		t.Errorf("body = %s, want the UPSTREAM_UNAVAILABLE error envelope", rec.Body)
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

// newServerWithNames wires a specific verbatim-name double, for the cases where
// the upstream path itself is under test.
func newServerWithNames(t *testing.T, query input.QueryService, names input.SpeciesNameQueryService) *httpapi.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return httpapi.NewServer(":0", httpapi.Deps{
		Health: stubHealth{ready: true}, Query: query, Names: names,
	}, logger, httpapi.Options{})
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
			"wcvp-1": {{HabitatTypeSummary: r22, Role: input.RoleDiagnostic, Syntaxa: syntaxa}},
		},
		bySyntaxon: map[string][]input.HabitatTypeSummary{"BRO-01A": {r22}},
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

func (f *fakeQueryService) HabitatType(_ context.Context, key domain.HabitatTypeKey, lang string) (input.HabitatTypeDetail, error) {
	f.habitatTypeCalls++
	f.lang = lang
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

func (f *fakeQueryService) SpeciesHabitatTypes(_ context.Context, conceptID, lang string) ([]input.HabitatTypeRole, error) {
	f.lang = lang
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

func (f *fakeQueryService) HabitatTypeSpecies(_ context.Context, key domain.HabitatTypeKey, role string) ([]input.SpeciesEntry, error) {
	f.speciesRoleFilter = role
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

// fakeNameQueryService doubles the one non-autark read path.
type fakeNameQueryService struct {
	query *fakeQueryService
	err   error
	// gotNames records what the handler passed down, so the dedupe and the
	// preserved input order can be asserted at the seam that matters.
	gotNames []string
}

func seededNameQueryService() *fakeNameQueryService {
	return &fakeNameQueryService{query: seededQueryService()}
}

func (f *fakeNameQueryService) SpeciesHabitatTypesByName(ctx context.Context, names []string, lang string) ([]input.NameResolution, error) {
	f.gotNames = append([]string(nil), names...)
	if f.err != nil {
		return nil, f.err
	}
	out := make([]input.NameResolution, 0, len(names))
	for _, n := range names {
		res := input.NameResolution{Verbatim: n, HabitatTypes: []input.HabitatTypeRole{}}
		if n == "Bromus erectus" {
			roles, err := f.query.SpeciesHabitatTypes(ctx, "wcvp-1", lang)
			if err != nil {
				return nil, err
			}
			res.ConceptID, res.Resolved, res.HabitatTypes = "wcvp-1", true, roles
		}
		out = append(out, res)
	}
	return out, nil
}
