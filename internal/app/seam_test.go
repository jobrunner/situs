package app_test

// The one test that composes the layers the way production does: a real sqlite
// index, seeded through the IngestTx port, read through application.QueryService,
// served through httpapi.NewServer. Every other test doubles at least one of
// these seams — the application tests use a fake repository, the HTTP tests a
// stub QueryService, the sqlite tests call the adapter directly — so this is the
// only place where preferredLabel meets the rows sqlite actually returns, the
// crosswalk inversion meets the rows as actually stored, and role bucketing
// meets the role strings the pipeline actually emits.
//
// It lives in internal/app because the depguard rules allow a _test.go file to
// wire adapters, application and domain together (the "test composition root").

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	httpapi "github.com/jobrunner/situs/internal/adapters/http"
	"github.com/jobrunner/situs/internal/adapters/sqlite"
	"github.com/jobrunner/situs/internal/application"
	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// seamServer opens an in-memory index, seeds it through the real IngestTx and
// returns a server wired over it exactly as the composition root does.
func seamServer(t *testing.T) *httpapi.Server {
	t.Helper()
	ctx := t.Context()

	db, err := sqlite.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:) = %v, want no error", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, ty := range []domain.Typology{
		{ID: "eunis@2021", Scheme: "eunis", Version: "2021"},
		{ID: "annex1", Scheme: "annex1"},
	} {
		if err := tx.UpsertTypology(ty); err != nil {
			t.Fatalf("UpsertTypology(%s): %v", ty.ID, err)
		}
	}

	level := 3
	priority := true
	eunis := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	annex := domain.HabitatTypeKey{Typology: "annex1", Code: "6510"}
	for _, h := range []domain.HabitatType{
		{Key: eunis, Level: &level, NameEN: "Mesic hay meadow"},
		{Key: annex, NameEN: "Lowland hay meadows", Priority: &priority},
	} {
		if err := tx.UpsertHabitatType(h); err != nil {
			t.Fatalf("UpsertHabitatType(%s): %v", h.Key, err)
		}
	}

	// Stored EUNIS -> Annex I as '<'; asked from the Annex I end it must read '>'.
	if err := tx.UpsertCrosswalk(domain.Crosswalk{
		From: eunis, To: annex, Qualifier: domain.QualifierNarrower,
	}); err != nil {
		t.Fatalf("UpsertCrosswalk: %v", err)
	}

	// The official German Annex I wording, plus the derived overlay on the EUNIS
	// type — the pair preferredLabel has to rank.
	for _, l := range []domain.Localization{
		{EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de", Field: "name",
			Value: "Magere Flachland-Mähwiesen", Source: "eur-lex", Provenance: "official"},
		{EntityType: "habitat_type", EntityKey: "eunis@2021:R22", Lang: "de", Field: "name",
			Value: "Magere Flachland-Mähwiesen", Source: "derived-annex1", Provenance: "derived"},
	} {
		if err := tx.UpsertLocalization(l); err != nil {
			t.Fatalf("UpsertLocalization(%s): %v", l.EntityKey, err)
		}
	}

	// The role strings the pipeline actually emits, so role bucketing is tested
	// against reality rather than against a test-only vocabulary.
	fidelity := 49.6
	concept := "wcvp-1"
	for _, r := range []domain.SpeciesRole{
		{Key: eunis, ConceptID: &concept, VerbatimName: "Bromus erectus",
			Role: "diagnostic", Fidelity: &fidelity},
		{Key: eunis, VerbatimName: "Unresolvable dubia", Role: "constant"},
	} {
		if err := tx.UpsertSpeciesRole(r); err != nil {
			t.Fatalf("UpsertSpeciesRole(%s): %v", r.VerbatimName, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return httpapi.NewServer(":0", httpapi.Deps{
		Health: application.NewHealthService(true),
		Query:  application.NewQueryService(db),
	}, logger, httpapi.Options{})
}

func seamGet(t *testing.T, srv *httpapi.Server, path string, into any) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d (%s), want 200", path, rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("decoding %s body %s: %v", path, rec.Body, err)
	}
}

// The German label must arrive additively, with its provenance, from the rows
// sqlite actually returns — and the English identity must survive.
func TestSeam_HabitatTypeInGermanCarriesTheDerivedLabelAndItsProvenance(t *testing.T) {
	srv := seamServer(t)

	var got input.HabitatTypeDetail
	seamGet(t, srv, "/v1/habitat-type/eunis@2021/R22?lang=de", &got)

	if got.NameEN != "Mesic hay meadow" {
		t.Errorf("name_en = %q, want the English identity to survive localization", got.NameEN)
	}
	if got.NameDE != "Magere Flachland-Mähwiesen" {
		t.Errorf("name_de = %q, want the derived German label", got.NameDE)
	}
	if got.NameDEProvenance != "derived" {
		t.Errorf("name_de_provenance = %q, want %q", got.NameDEProvenance, "derived")
	}

	// Role bucketing over the pipeline's own role strings.
	if len(got.Species["diagnostic"]) != 1 || got.Species["diagnostic"][0].VerbatimName != "Bromus erectus" {
		t.Errorf("species[diagnostic] = %+v, want the one diagnostic species", got.Species["diagnostic"])
	}
	if len(got.Species["constant"]) != 1 || got.Species["constant"][0].ConceptID != "" {
		t.Errorf("species[constant] = %+v, want the unresolved name kept without a concept id",
			got.Species["constant"])
	}
	if _, ok := got.Species["dominant"]; !ok {
		t.Error("species has no dominant bucket; an empty bucket is information, absence is not")
	}
}

// One stored crosswalk row answers both ends. Asked from the Annex I side, the
// qualifier must be inverted against the rows as actually stored.
func TestSeam_CrosswalkReadsCorrectlyFromEitherEnd(t *testing.T) {
	srv := seamServer(t)

	var eunis input.HabitatTypeDetail
	seamGet(t, srv, "/v1/habitat-type/eunis@2021/R22", &eunis)
	if len(eunis.Crosswalks) != 1 {
		t.Fatalf("crosswalks = %+v, want exactly one", eunis.Crosswalks)
	}
	if eunis.Crosswalks[0].Qualifier != domain.QualifierNarrower {
		t.Errorf("qualifier from the EUNIS end = %q, want %q as stored",
			eunis.Crosswalks[0].Qualifier, domain.QualifierNarrower)
	}
	if eunis.Crosswalks[0].Code != "6510" {
		t.Errorf("crosswalk target = %q, want the Annex I code", eunis.Crosswalks[0].Code)
	}

	var annex input.HabitatTypeDetail
	seamGet(t, srv, "/v1/habitat-type/annex1/6510", &annex)
	if len(annex.Crosswalks) != 1 {
		t.Fatalf("crosswalks = %+v, want exactly one", annex.Crosswalks)
	}
	if annex.Crosswalks[0].Qualifier != domain.QualifierBroader {
		t.Errorf("qualifier from the Annex I end = %q, want it inverted to %q",
			annex.Crosswalks[0].Qualifier, domain.QualifierBroader)
	}
	if annex.Crosswalks[0].Code != "R22" {
		t.Errorf("crosswalk target = %q, want the EUNIS code", annex.Crosswalks[0].Code)
	}
	if annex.Priority == nil || !*annex.Priority {
		t.Errorf("priority = %v, want true for this Annex I type", annex.Priority)
	}
}

// The excursion app's main question, end to end over a real index.
func TestSeam_SpeciesHabitatTypesAnswersFromTheRealIndex(t *testing.T) {
	srv := seamServer(t)

	var got []input.HabitatTypeRole
	seamGet(t, srv, "/v1/species/wcvp-1/habitat-types", &got)

	if len(got) != 1 {
		t.Fatalf("got %d habitat types, want the one R22 plays a role in", len(got))
	}
	if got[0].Code != "R22" || got[0].Role != "diagnostic" {
		t.Errorf("got %+v, want R22 with role diagnostic", got[0])
	}
	if got[0].Fidelity == nil || *got[0].Fidelity != 49.6 {
		t.Errorf("fidelity = %v, want the stored 49.6 to survive the round trip", got[0].Fidelity)
	}
}
