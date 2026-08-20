package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
	"github.com/jobrunner/situs/internal/ports/output"
)

var (
	queryR22 = domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	queryR99 = domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R99"}
	queryLRT = domain.HabitatTypeKey{Typology: "annex1", Code: "6510"}
	query212 = domain.HabitatTypeKey{Typology: "eunis@2012", Code: "E2.2"}
)

// seedQueryRepo builds the little world the query tests ask about: R22 with one
// resolved and one unresolved species, a syntaxon, an outgoing '=' crosswalk to
// Annex I and an incoming '<' crosswalk from the 2012 fassung.
func seedQueryRepo() *fakeRepo {
	repo := newFakeRepo()
	level := 3
	priority := true
	concept := "wcvp-1"
	fidelity := 49.6

	repo.typologies = []domain.Typology{
		{ID: "eunis@2021", Scheme: "eunis", Version: "2021"},
		{ID: "eunis@2012", Scheme: "eunis", Version: "2012"},
		{ID: "annex1", Scheme: "annex1"},
	}
	repo.types = []domain.HabitatType{
		{Key: queryR22, Level: &level, NameEN: "Low and medium altitude hay meadow"},
		{Key: queryR99, NameEN: "Lonely type"},
		{Key: queryLRT, NameEN: "Lowland hay meadows", Priority: &priority},
		{Key: query212, NameEN: "Hay meadow (2012)"},
	}
	repo.crosswalks = []domain.Crosswalk{
		{From: queryR22, To: queryLRT, Qualifier: domain.QualifierSame},
		{From: query212, To: queryR22, Qualifier: domain.QualifierNarrower},
	}
	repo.syntaxa = []domain.Syntaxon{{ID: "BRO-01A", Rank: "alliance", Name: "Bromion erecti"}}
	repo.syntaxaLinks = []struct {
		key        domain.HabitatTypeKey
		syntaxonID string
	}{{key: queryR22, syntaxonID: "BRO-01A"}}
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: queryR22, ConceptID: &concept, VerbatimName: "Bromus erectus", Role: "diagnostic", Fidelity: &fidelity},
		{Key: queryR22, VerbatimName: "Unresolvable dubia", Role: "constant"},
	}
	repo.localizations = []domain.Localization{{
		EntityType: "habitat_type", EntityKey: queryR22.String(), Lang: "de", Field: "name",
		Value: "Magere Flachland-Mähwiese", Source: "derived-annex1", Provenance: "derived",
	}}
	return repo
}

func TestQueryService_HabitatTypeCarriesSpeciesSyntaxaAndCrosswalks(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	got, err := q.HabitatType(context.Background(), queryR22, "en", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.NameEN != "Low and medium altitude hay meadow" || got.Level == nil || *got.Level != 3 {
		t.Errorf("got %+v, want the seeded name and level", got.HabitatTypeSummary)
	}
	if got.NameDE != "" || got.NameDEProvenance != "" {
		t.Errorf("name_de = %q — an English request must not carry the overlay", got.NameDE)
	}
	if len(got.Species["diagnostic"]) != 1 || len(got.Species["constant"]) != 1 {
		t.Errorf("species = %+v, want one diagnostic and one constant entry", got.Species)
	}
	if len(got.Species["dominant"]) != 0 {
		t.Errorf("species[dominant] = %+v, want an empty bucket", got.Species["dominant"])
	}
	if got.Species["constant"][0].ConceptID != "" {
		t.Error("the unresolved species must arrive without a concept id, not with an invented one")
	}
	if len(got.Syntaxa) != 1 || got.Syntaxa[0].ID != "BRO-01A" {
		t.Errorf("syntaxa = %+v, want the linked alliance", got.Syntaxa)
	}
	if len(got.Crosswalks) != 2 {
		t.Fatalf("crosswalks = %+v, want both directions", got.Crosswalks)
	}
}

// The stored row eunis@2012:E2.2 -< R22 must read, from R22's side, as R22 being
// BROADER than E2.2 — otherwise the answer inverts the meaning of the source.
func TestQueryService_IncomingCrosswalkIsInverted(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	got, err := q.HabitatType(context.Background(), queryR22, "en", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	byCode := map[string]input.CrosswalkRef{}
	for _, c := range got.Crosswalks {
		byCode[c.Code] = c
	}
	if ref := byCode["6510"]; ref.Typology != "annex1" || ref.Qualifier != domain.QualifierSame {
		t.Errorf("outgoing crosswalk = %+v, want annex1:6510 with '='", ref)
	}
	if ref := byCode["E2.2"]; ref.Qualifier != domain.QualifierBroader {
		t.Errorf("incoming crosswalk = %+v, want the inverted '>' qualifier", ref)
	}
}

// Localization is an overlay: name_en stays the identity, name_de is added and
// says where it came from.
func TestQueryService_GermanLabelIsAdditiveAndCarriesProvenance(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	got, err := q.HabitatType(context.Background(), queryR22, "de", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.NameEN != "Low and medium altitude hay meadow" {
		t.Errorf("name_en = %q, want the English name kept", got.NameEN)
	}
	if got.NameDE != "Magere Flachland-Mähwiese" || got.NameDEProvenance != "derived" {
		t.Errorf("name_de/provenance = %q/%q, want the derived overlay", got.NameDE, got.NameDEProvenance)
	}
}

// An official label outranks a curated one and both outrank a derived one, so
// the served wording never depends on row order.
func TestQueryService_PreferredLabelOrdersOfficialCuratedDerived(t *testing.T) {
	for name, tc := range map[string]struct {
		provenances []string
		want        string
	}{
		"official wins":            {provenances: []string{"derived", "official", "curated"}, want: "official"},
		"curated beats derived":    {provenances: []string{"derived", "curated"}, want: "curated"},
		"derived is served lastly": {provenances: []string{"derived"}, want: "derived"},
	} {
		t.Run(name, func(t *testing.T) {
			repo := seedQueryRepo()
			repo.localizations = nil
			for _, p := range tc.provenances {
				repo.localizations = append(repo.localizations, domain.Localization{
					EntityType: "habitat_type", EntityKey: queryR22.String(), Lang: "de", Field: "name",
					Value: p + " label", Source: p, Provenance: p,
				})
			}

			got, err := NewQueryService(repo).HabitatType(context.Background(), queryR22, "de", input.AreaFilter{})
			if err != nil {
				t.Fatalf("HabitatType: %v", err)
			}
			if got.NameDEProvenance != tc.want || got.NameDE != tc.want+" label" {
				t.Errorf("name_de/provenance = %q/%q, want the %s label", got.NameDE, got.NameDEProvenance, tc.want)
			}
		})
	}
}

func TestQueryService_NoGermanLabelLeavesTheOverlayEmpty(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	got, err := q.HabitatType(context.Background(), queryR99, "de", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.NameDE != "" || got.NameDEProvenance != "" {
		t.Errorf("name_de/provenance = %q/%q, want both empty when nothing is localized",
			got.NameDE, got.NameDEProvenance)
	}
}

// A type with no crosswalks and no syntaxa is the normal case, not an error, and
// its lists must be empty rather than nil.
func TestQueryService_EmptyListsAreEmptyNotNil(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	got, err := q.HabitatType(context.Background(), queryR99, "en", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.Crosswalks == nil || len(got.Crosswalks) != 0 {
		t.Errorf("crosswalks = %+v, want an empty, non-nil slice", got.Crosswalks)
	}
	if got.Syntaxa == nil || len(got.Syntaxa) != 0 {
		t.Errorf("syntaxa = %+v, want an empty, non-nil slice", got.Syntaxa)
	}
	for _, role := range []string{input.RoleDiagnostic, input.RoleConstant, input.RoleDominant} {
		if got.Species[role] == nil {
			t.Errorf("species[%q] is nil, want an empty bucket", role)
		}
	}
}

func TestQueryService_UnknownTypologyIsDistinctFromUnknownCode(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	_, err := q.HabitatType(context.Background(), domain.HabitatTypeKey{Typology: "bogus@1", Code: "R22"}, "en", input.AreaFilter{})
	if !errors.Is(err, input.ErrUnknownTypology) {
		t.Errorf("unknown typology error = %v, want input.ErrUnknownTypology", err)
	}
	if errors.Is(err, input.ErrNotFound) {
		t.Error("an unknown typology must not also report ErrNotFound — the status codes differ")
	}

	_, err = q.HabitatType(context.Background(), domain.HabitatTypeKey{Typology: "eunis@2021", Code: "NOPE"}, "en", input.AreaFilter{})
	if !errors.Is(err, input.ErrNotFound) {
		t.Errorf("unknown code error = %v, want input.ErrNotFound", err)
	}
}

func TestQueryService_SpeciesHabitatTypes(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	got, err := q.SpeciesHabitatTypes(context.Background(), "wcvp-1", "de", input.AreaFilter{})
	if err != nil {
		t.Fatalf("SpeciesHabitatTypes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %+v, want the one habitat type the concept has a role in", got)
	}
	if got[0].Role != "diagnostic" || got[0].Code != "R22" {
		t.Errorf("got %+v, want R22 as diagnostic", got[0])
	}
	if got[0].Fidelity == nil || *got[0].Fidelity != 49.6 {
		t.Errorf("fidelity = %v, want the seeded 49.6", got[0].Fidelity)
	}
	if got[0].NameDE == "" {
		t.Error("name_de missing — lang must reach the species answer too")
	}
	if len(got[0].Syntaxa) != 1 {
		t.Errorf("syntaxa = %+v, want the type's syntaxa", got[0].Syntaxa)
	}
}

func TestQueryService_UnknownConceptIsNotFound(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	if _, err := q.SpeciesHabitatTypes(context.Background(), "wcvp-nope", "en", input.AreaFilter{}); !errors.Is(err, input.ErrNotFound) {
		t.Errorf("error = %v, want input.ErrNotFound", err)
	}
}

// A species role pointing at a habitat type the index does not carry is an
// inconsistent index. It must be reported, not smoothed over with an empty name
// and not turned into a 404 for the whole species.
func TestQueryService_DanglingSpeciesRoleIsReportedAsInconsistency(t *testing.T) {
	repo := seedQueryRepo()
	concept := "wcvp-dangling"
	repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
		Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "GONE"}, ConceptID: &concept,
		VerbatimName: "Dangling reference", Role: "diagnostic",
	})

	_, err := NewQueryService(repo).SpeciesHabitatTypes(context.Background(), concept, "en", input.AreaFilter{})
	if err == nil {
		t.Fatal("SpeciesHabitatTypes = nil error, want the inconsistency reported")
	}
	if errors.Is(err, input.ErrNotFound) {
		t.Errorf("error = %v, want an internal inconsistency, not a NOT_FOUND for the species", err)
	}
}

func TestQueryService_HabitatTypeSpeciesFiltersByRole(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	all, err := q.HabitatTypeSpecies(context.Background(), queryR22, "", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatTypeSpecies(all): %v", err)
	}
	if len(all) != 2 {
		t.Errorf("got %+v, want every species when no role is given", all)
	}

	diagnostic, err := q.HabitatTypeSpecies(context.Background(), queryR22, "diagnostic", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatTypeSpecies(diagnostic): %v", err)
	}
	if len(diagnostic) != 1 || diagnostic[0].VerbatimName != "Bromus erectus" {
		t.Errorf("got %+v, want only the diagnostic species", diagnostic)
	}

	none, err := q.HabitatTypeSpecies(context.Background(), queryR99, "", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatTypeSpecies(R99): %v", err)
	}
	if none == nil || len(none) != 0 {
		t.Errorf("got %+v, want an empty, non-nil slice", none)
	}
}

func TestQueryService_HabitatTypeSpeciesRejectsUnknownTypologyAndCode(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	_, err := q.HabitatTypeSpecies(context.Background(), domain.HabitatTypeKey{Typology: "bogus@1", Code: "R22"}, "", input.AreaFilter{})
	if !errors.Is(err, input.ErrUnknownTypology) {
		t.Errorf("error = %v, want input.ErrUnknownTypology", err)
	}
	_, err = q.HabitatTypeSpecies(context.Background(), domain.HabitatTypeKey{Typology: "eunis@2021", Code: "NOPE"}, "", input.AreaFilter{})
	if !errors.Is(err, input.ErrNotFound) {
		t.Errorf("error = %v, want input.ErrNotFound", err)
	}
}

func TestQueryService_SyntaxonHabitatTypes(t *testing.T) {
	q := NewQueryService(seedQueryRepo())

	got, err := q.SyntaxonHabitatTypes(context.Background(), "BRO-01A", "de")
	if err != nil {
		t.Fatalf("SyntaxonHabitatTypes: %v", err)
	}
	if len(got) != 1 || got[0].Code != "R22" {
		t.Fatalf("got %+v, want the single linked type", got)
	}
	if got[0].NameEN == "" || got[0].NameDE == "" {
		t.Errorf("got %+v, want both the English identity and the German overlay", got[0])
	}

	if _, err := q.SyntaxonHabitatTypes(context.Background(), "NOPE", "en"); !errors.Is(err, input.ErrNotFound) {
		t.Errorf("unknown syntaxon error = %v, want input.ErrNotFound", err)
	}
}

// A syntaxon that exists but is linked to nothing answers with an empty list —
// that is different from not existing at all.
func TestQueryService_SyntaxonWithoutLinksIsAnEmptyList(t *testing.T) {
	repo := seedQueryRepo()
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{ID: "UNLINKED", Rank: "order", Name: "Nothing links here"})

	got, err := NewQueryService(repo).SyntaxonHabitatTypes(context.Background(), "UNLINKED", "en")
	if err != nil {
		t.Fatalf("SyntaxonHabitatTypes: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got %+v, want an empty, non-nil list", got)
	}
}

func TestQueryService_SyntaxonLinkedToUnknownTypeIsReportedAsInconsistency(t *testing.T) {
	repo := seedQueryRepo()
	repo.syntaxaLinks = append(repo.syntaxaLinks, struct {
		key        domain.HabitatTypeKey
		syntaxonID string
	}{key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "GONE"}, syntaxonID: "BRO-01A"})

	_, err := NewQueryService(repo).SyntaxonHabitatTypes(context.Background(), "BRO-01A", "en")
	if err == nil {
		t.Fatal("SyntaxonHabitatTypes = nil error, want the inconsistency reported")
	}
	if errors.Is(err, input.ErrNotFound) {
		t.Errorf("error = %v, want an internal inconsistency, not a NOT_FOUND", err)
	}
}

// Every repository failure must surface, never be answered with a partial result.
func TestQueryService_RepositoryFailuresSurface(t *testing.T) {
	boom := errors.New("boom")
	ctx := context.Background()

	cases := map[string]struct {
		arrange func(r *fakeRepo)
		call    func(q *QueryService) error
	}{
		"Typology fails": {
			arrange: func(r *fakeRepo) { r.typologyErr = boom },
			call: func(q *QueryService) error {
				_, err := q.HabitatType(ctx, queryR22, "en", input.AreaFilter{})
				return err
			},
		},
		"HabitatType fails": {
			arrange: func(r *fakeRepo) { r.habitatTypeErr = boom },
			call: func(q *QueryService) error {
				_, err := q.HabitatType(ctx, queryR22, "en", input.AreaFilter{})
				return err
			},
		},
		"SpeciesRoles fails": {
			arrange: func(r *fakeRepo) { r.speciesRolesErr = boom },
			call: func(q *QueryService) error {
				_, err := q.HabitatType(ctx, queryR22, "en", input.AreaFilter{})
				return err
			},
		},
		"Syntaxa fails": {
			arrange: func(r *fakeRepo) { r.syntaxaErr = boom },
			call: func(q *QueryService) error {
				_, err := q.HabitatType(ctx, queryR22, "en", input.AreaFilter{})
				return err
			},
		},
		"Crosswalks fails": {
			arrange: func(r *fakeRepo) { r.crosswalksErr = boom },
			call: func(q *QueryService) error {
				_, err := q.HabitatType(ctx, queryR22, "en", input.AreaFilter{})
				return err
			},
		},
		"Localization fails": {
			arrange: func(r *fakeRepo) { r.localizationErr = boom },
			call: func(q *QueryService) error {
				_, err := q.HabitatType(ctx, queryR22, "de", input.AreaFilter{})
				return err
			},
		},
		"SpeciesRolesByConcept fails": {
			arrange: func(r *fakeRepo) { r.speciesRolesErr = boom },
			call: func(q *QueryService) error {
				_, err := q.SpeciesHabitatTypes(ctx, "wcvp-1", "en", input.AreaFilter{})
				return err
			},
		},
		"KnownAreaCodes fails when an area filter is active": {
			arrange: func(r *fakeRepo) { r.areasErr = boom },
			call: func(q *QueryService) error {
				_, err := q.HabitatTypeSpecies(ctx, queryR22, "", input.AreaFilter{Code: "GER"})
				return err
			},
		},
		"KnownAreaCodes fails on the batch path": {
			arrange: func(r *fakeRepo) { r.areasErr = boom },
			call: func(q *QueryService) error {
				_, err := q.SpeciesSetHabitatTypes(ctx, []string{"wcvp:concept:1"}, "en",
					input.AreaFilter{Code: "GER"})
				return err
			},
		},
		// An index failure must fail the batch, not quietly become "no facts for
		// this concept" — that would report a real outage as a normal answer.
		"the index fails on the batch path": {
			arrange: func(r *fakeRepo) { r.speciesRolesErr = boom },
			call: func(q *QueryService) error {
				_, err := q.SpeciesSetHabitatTypes(ctx, []string{"wcvp:concept:1"}, "en", input.AreaFilter{})
				return err
			},
		},
		"Syntaxa fails on the species path": {
			arrange: func(r *fakeRepo) { r.syntaxaErr = boom },
			call: func(q *QueryService) error {
				_, err := q.SpeciesHabitatTypes(ctx, "wcvp-1", "en", input.AreaFilter{})
				return err
			},
		},
		"Localization fails on the species path": {
			arrange: func(r *fakeRepo) { r.localizationErr = boom },
			call: func(q *QueryService) error {
				_, err := q.SpeciesHabitatTypes(ctx, "wcvp-1", "de", input.AreaFilter{})
				return err
			},
		},
		"Localization fails on the syntaxon path": {
			arrange: func(r *fakeRepo) { r.localizationErr = boom },
			call:    func(q *QueryService) error { _, err := q.SyntaxonHabitatTypes(ctx, "BRO-01A", "de"); return err },
		},
		"Syntaxon fails": {
			arrange: func(r *fakeRepo) { r.syntaxonErr = boom },
			call:    func(q *QueryService) error { _, err := q.SyntaxonHabitatTypes(ctx, "BRO-01A", "en"); return err },
		},
		"keys for syntaxon fail": {
			arrange: func(r *fakeRepo) { r.syntaxonKeysErr = boom },
			call:    func(q *QueryService) error { _, err := q.SyntaxonHabitatTypes(ctx, "BRO-01A", "en"); return err },
		},
		"HabitatType fails on the species-list path": {
			arrange: func(r *fakeRepo) { r.habitatTypeErr = boom },
			call: func(q *QueryService) error {
				_, err := q.HabitatTypeSpecies(ctx, queryR22, "", input.AreaFilter{})
				return err
			},
		},
		"SpeciesRoles fails on the species-list path": {
			arrange: func(r *fakeRepo) { r.speciesRolesErr = boom },
			call: func(q *QueryService) error {
				_, err := q.HabitatTypeSpecies(ctx, queryR22, "", input.AreaFilter{})
				return err
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := seedQueryRepo()
			tc.arrange(repo)

			err := tc.call(NewQueryService(repo))
			if !errors.Is(err, boom) {
				t.Errorf("error = %v, want it to wrap the repository failure", err)
			}
		})
	}
}

// The read side of fakeRepo. It is deliberately a naive scan over the seeded
// slices: a query test must not depend on a second implementation of the
// filtering it is checking.

func (r *fakeRepo) Typology(_ context.Context, id domain.TypologyID) (domain.Typology, error) {
	if r.typologyErr != nil {
		return domain.Typology{}, r.typologyErr
	}
	for _, t := range r.typologies {
		if t.ID == id {
			return t, nil
		}
	}
	return domain.Typology{}, fmt.Errorf("fakeRepo: typology %s: %w", id, output.ErrNotFound)
}

func (r *fakeRepo) Crosswalks(_ context.Context, key domain.HabitatTypeKey) ([]domain.Crosswalk, error) {
	if r.crosswalksErr != nil {
		return nil, r.crosswalksErr
	}
	out := []domain.Crosswalk{}
	for _, c := range r.crosswalks {
		if c.From == key || c.To == key {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *fakeRepo) SpeciesRoles(_ context.Context, key domain.HabitatTypeKey, role string) ([]domain.SpeciesRole, error) {
	if r.speciesRolesErr != nil {
		return nil, r.speciesRolesErr
	}
	out := []domain.SpeciesRole{}
	for _, s := range r.speciesRoles {
		if s.Key == key && (role == "" || s.Role == role) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *fakeRepo) SpeciesRolesByConcept(_ context.Context, conceptID string) ([]domain.SpeciesRole, error) {
	if r.speciesRolesErr != nil {
		return nil, r.speciesRolesErr
	}
	out := []domain.SpeciesRole{}
	for _, s := range r.speciesRoles {
		if s.ConceptID != nil && *s.ConceptID == conceptID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *fakeRepo) Syntaxon(_ context.Context, id string) (domain.Syntaxon, error) {
	if r.syntaxonErr != nil {
		return domain.Syntaxon{}, r.syntaxonErr
	}
	for _, s := range r.syntaxa {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.Syntaxon{}, fmt.Errorf("fakeRepo: syntaxon %q: %w", id, output.ErrNotFound)
}

func (r *fakeRepo) Syntaxa(_ context.Context, key domain.HabitatTypeKey) ([]domain.Syntaxon, error) {
	if r.syntaxaErr != nil {
		return nil, r.syntaxaErr
	}
	out := []domain.Syntaxon{}
	for _, link := range r.syntaxaLinks {
		if link.key != key {
			continue
		}
		for _, s := range r.syntaxa {
			if s.ID == link.syntaxonID {
				out = append(out, s)
			}
		}
	}
	return out, nil
}

func (r *fakeRepo) HabitatTypeKeysForSyntaxon(_ context.Context, syntaxonID string) ([]domain.HabitatTypeKey, error) {
	if r.syntaxonKeysErr != nil {
		return nil, r.syntaxonKeysErr
	}
	out := []domain.HabitatTypeKey{}
	for _, link := range r.syntaxaLinks {
		if link.syntaxonID == syntaxonID {
			out = append(out, link.key)
		}
	}
	return out, nil
}

func TestHabitatTypeSpecies_MarksAreaInThreeStates(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	here, elsewhere := "wcvp:concept:here", "wcvp:concept:elsewhere"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &here, VerbatimName: "Here", Role: "diagnostic"},
		{Key: key, ConceptID: &elsewhere, VerbatimName: "Elsewhere", Role: "diagnostic"},
		{Key: key, ConceptID: nil, VerbatimName: "Moss", Role: "diagnostic"},
	}
	repo.distribution = []fakeDistribution{
		{ConceptID: here, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
		{ConceptID: elsewhere, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "FRA"}},
	}

	got, err := NewQueryService(repo).HabitatTypeSpecies(context.Background(), key, "",
		input.AreaFilter{Code: "GER"})
	if err != nil {
		t.Fatalf("HabitatTypeSpecies: %v", err)
	}
	byName := map[string]*bool{}
	for _, s := range got {
		byName[s.VerbatimName] = s.InArea
	}
	if byName["Here"] == nil || !*byName["Here"] {
		t.Errorf("Here: in_area = %v, want true", byName["Here"])
	}
	if byName["Elsewhere"] == nil || *byName["Elsewhere"] {
		t.Errorf("Elsewhere: in_area = %v, want false", byName["Elsewhere"])
	}
	if byName["Moss"] != nil {
		t.Errorf("Moss: in_area = %v, want nil (no concept, so not knowable)", byName["Moss"])
	}
}

// only_in_area removes the definite absences and keeps the unknowns: a list
// that silently loses what it cannot judge is dishonestly clean.
func TestHabitatTypeSpecies_OnlyInAreaKeepsTheUnknowns(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	here, elsewhere := "wcvp:concept:here", "wcvp:concept:elsewhere"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &here, VerbatimName: "Here", Role: "diagnostic"},
		{Key: key, ConceptID: &elsewhere, VerbatimName: "Elsewhere", Role: "diagnostic"},
		{Key: key, ConceptID: nil, VerbatimName: "Moss", Role: "diagnostic"},
	}
	repo.distribution = []fakeDistribution{
		{ConceptID: here, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
		{ConceptID: elsewhere, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "FRA"}},
	}

	got, err := NewQueryService(repo).HabitatTypeSpecies(context.Background(), key, "",
		input.AreaFilter{Code: "GER", OnlyInArea: true})
	if err != nil {
		t.Fatalf("HabitatTypeSpecies: %v", err)
	}
	names := map[string]bool{}
	for _, s := range got {
		names[s.VerbatimName] = true
	}
	if !names["Here"] || !names["Moss"] {
		t.Errorf("names = %v, want Here and Moss (unknown stays)", names)
	}
	if names["Elsewhere"] {
		t.Error("Elsewhere survived only_in_area=true")
	}
}

func TestHabitatTypeSpecies_UnknownAreaIsAnError(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	repo.distribution = []fakeDistribution{
		{ConceptID: "wcvp:concept:1", Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}

	_, err := NewQueryService(repo).HabitatTypeSpecies(context.Background(), key, "",
		input.AreaFilter{Code: "NOPE"})
	if !errors.Is(err, input.ErrUnknownArea) {
		t.Errorf("err = %v, want ErrUnknownArea — a list of false would hide the typo", err)
	}
}

func TestHabitatTypeSpecies_WithoutAreaThereIsNoField(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	id := "wcvp:concept:1"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &id, VerbatimName: "A", Role: "diagnostic"},
	}

	got, err := NewQueryService(repo).HabitatTypeSpecies(context.Background(), key, "",
		input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatTypeSpecies: %v", err)
	}
	if len(got) != 1 || got[0].InArea != nil {
		t.Errorf("in_area = %v, want nil without an area filter", got[0].InArea)
	}

	// The requirement is on the wire, not on the Go zero value: omitempty on a
	// nil *bool must drop the key entirely, not serve a literal null.
	raw, err := json.Marshal(got[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(raw, []byte(`"in_area"`)) {
		t.Errorf("body = %s, want no in_area key at all without an area filter", raw)
	}
}

// HabitatType must carry the same three-state marking as HabitatTypeSpecies —
// across every role bucket, not just the flat species list.
func TestHabitatType_MarksAreaAcrossRoleBuckets(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	here := "wcvp:concept:here"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &here, VerbatimName: "Here", Role: "diagnostic"},
	}
	repo.distribution = []fakeDistribution{
		{ConceptID: here, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}

	got, err := NewQueryService(repo).HabitatType(context.Background(), key, "en", input.AreaFilter{Code: "GER"})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	entries := got.Species["diagnostic"]
	if len(entries) != 1 || entries[0].InArea == nil || !*entries[0].InArea {
		t.Errorf("species[diagnostic] = %+v, want the entry marked in_area=true", entries)
	}
}

func TestHabitatType_UnknownAreaIsAnError(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	repo.distribution = []fakeDistribution{
		{ConceptID: "wcvp:concept:1", Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}

	_, err := NewQueryService(repo).HabitatType(context.Background(), key, "en", input.AreaFilter{Code: "NOPE"})
	if !errors.Is(err, input.ErrUnknownArea) {
		t.Errorf("err = %v, want ErrUnknownArea", err)
	}
}

// SpeciesHabitatTypes marks the single queried concept's own area membership,
// and only_in_area empties the answer outright when it is definitely absent —
// but a definite absence without only_in_area stays visible.
func TestSpeciesHabitatTypes_MarksAndFiltersByArea(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	concept := "wcvp:concept:1"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &concept, VerbatimName: "Here", Role: "diagnostic"},
	}
	repo.distribution = []fakeDistribution{
		{ConceptID: concept, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
		// A row for a different concept, so FRA is a known area code the
		// queried concept simply has no row for — a definite absence.
		{ConceptID: "wcvp:concept:other", Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "FRA"}},
	}
	q := NewQueryService(repo)

	got, err := q.SpeciesHabitatTypes(context.Background(), concept, "en", input.AreaFilter{Code: "GER"})
	if err != nil {
		t.Fatalf("SpeciesHabitatTypes: %v", err)
	}
	if len(got) != 1 || got[0].InArea == nil || !*got[0].InArea {
		t.Errorf("got %+v, want in_area=true", got)
	}

	got, err = q.SpeciesHabitatTypes(context.Background(), concept, "en",
		input.AreaFilter{Code: "FRA", OnlyInArea: true})
	if err != nil {
		t.Fatalf("SpeciesHabitatTypes: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want an empty list — the concept is definitely absent from FRA", got)
	}

	got, err = q.SpeciesHabitatTypes(context.Background(), concept, "en", input.AreaFilter{Code: "FRA"})
	if err != nil {
		t.Fatalf("SpeciesHabitatTypes: %v", err)
	}
	if len(got) != 1 || got[0].InArea == nil || *got[0].InArea {
		t.Errorf("got %+v, want in_area=false kept without only_in_area", got)
	}
}

func TestSpeciesHabitatTypes_UnknownAreaIsAnError(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	concept := "wcvp:concept:1"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &concept, VerbatimName: "Here", Role: "diagnostic"},
	}
	repo.distribution = []fakeDistribution{
		{ConceptID: concept, Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}},
	}

	_, err := NewQueryService(repo).SpeciesHabitatTypes(context.Background(), concept, "en", input.AreaFilter{Code: "NOPE"})
	if !errors.Is(err, input.ErrUnknownArea) {
		t.Errorf("err = %v, want ErrUnknownArea", err)
	}
}

// inArea's four cases as a direct unit test, including the one no end-to-end
// test reaches: a concept present in the request but absent from the areas
// map entirely (distribution rows for other concepts, none for this one).
func TestInArea_ThreeStatesDirectly(t *testing.T) {
	areas := map[string][]string{"wcvp-1": {"GER"}}

	if got := inArea(nil, "wcvp-1", "GER"); got != nil {
		t.Errorf("no filter active (areas nil): got %v, want nil", got)
	}
	if got := inArea(areas, "", "GER"); got != nil {
		t.Errorf("no concept id: got %v, want nil", got)
	}
	if got := inArea(areas, "wcvp-2", "GER"); got != nil {
		t.Errorf("concept absent from the areas map: got %v, want nil", got)
	}
	if got := inArea(areas, "wcvp-1", "GER"); got == nil || !*got {
		t.Errorf("concept present, area matches: got %v, want true", got)
	}
	if got := inArea(areas, "wcvp-1", "FRA"); got == nil || *got {
		t.Errorf("concept present, area does not match: got %v, want false", got)
	}
}

func TestSpeciesSetHabitatTypes_OneEntryPerInputInOrder(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	known := "wcvp:concept:known"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &known, VerbatimName: "A", Role: "diagnostic"},
	}

	in := []string{known, "wcvp:concept:nofacts", "cdm:concept:other", known}
	got, err := NewQueryService(repo).SpeciesSetHabitatTypes(context.Background(), in, "", input.AreaFilter{})
	if err != nil {
		t.Fatalf("SpeciesSetHabitatTypes: %v", err)
	}
	if len(got) != len(in) {
		t.Fatalf("got %d entries, want %d — one per input, duplicates included", len(got), len(in))
	}
	for i, want := range in {
		if got[i].ConceptID != want {
			t.Errorf("entry %d = %q, want %q (input order)", i, got[i].ConceptID, want)
		}
	}
	if !got[0].Known || len(got[0].HabitatTypes) != 1 {
		t.Errorf("entry 0 = %+v, want known with one habitat type", got[0])
	}
	if got[3].ConceptID != known || !got[3].Known {
		t.Errorf("entry 3 = %+v, want the duplicate answered too", got[3])
	}
}

// The two reasons are different diagnoses: a wrong backbone is the caller's
// mistake, a concept without facts is the data's limit. One label for both
// sends people looking in the wrong place.
func TestSpeciesSetHabitatTypes_DistinguishesTheTwoReasons(t *testing.T) {
	repo := newFakeRepo()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	repo.types = []domain.HabitatType{{Key: key, NameEN: "Hay meadow"}}
	known := "wcvp:concept:known"
	repo.speciesRoles = []domain.SpeciesRole{
		{Key: key, ConceptID: &known, VerbatimName: "A", Role: "diagnostic"},
	}

	got, err := NewQueryService(repo).SpeciesSetHabitatTypes(context.Background(),
		[]string{"cdm:concept:x", "wcvp:concept:nofacts"}, "", input.AreaFilter{})
	if err != nil {
		t.Fatalf("SpeciesSetHabitatTypes: %v", err)
	}
	if got[0].Known || got[0].Reason != "unknown_backbone" {
		t.Errorf("entry 0 = %+v, want unknown_backbone", got[0])
	}
	if got[1].Known || got[1].Reason != "unknown_concept" {
		t.Errorf("entry 1 = %+v, want unknown_concept", got[1])
	}
}
