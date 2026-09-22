package sqlite

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

var (
	r22   = domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	r99   = domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R99"}
	lrt   = domain.HabitatTypeKey{Typology: "annex1", Code: "6510"}
	e2012 = domain.HabitatTypeKey{Typology: "eunis@2012", Code: "E2.2"}
)

// seedReadFixture fills a fresh index with the small world the read tests ask
// questions about: two EUNIS types, one Annex I type, crosswalks in both
// directions, one syntaxon linked to R22 and three species in different roles —
// one of them unresolved.
func seedReadFixture(t *testing.T, db *DB) {
	t.Helper()
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	level := 3
	priority := true
	concept := "wcvp-1"
	fidelity := 49.6

	for _, ty := range []domain.Typology{
		{ID: "eunis@2021", Scheme: "eunis", Version: "2021"},
		{ID: "annex1", Scheme: "annex1"},
	} {
		if err := tx.UpsertTypology(ty); err != nil {
			t.Fatalf("UpsertTypology(%s): %v", ty.ID, err)
		}
	}
	for _, h := range []domain.HabitatType{
		{Key: r22, Level: &level, NameEN: "Low and medium altitude hay meadow"},
		{Key: r99, NameEN: "Lonely type"},
		{Key: lrt, NameEN: "Lowland hay meadows", Priority: &priority},
	} {
		if err := tx.UpsertHabitatType(h); err != nil {
			t.Fatalf("UpsertHabitatType(%s): %v", h.Key, err)
		}
	}
	// R22 -> annex1:6510 ('='), and eunis@2012:E2.2 -> R22 ('<'): the second row
	// is stored pointing AT R22, so a query for R22 must still find it.
	for _, c := range []domain.Crosswalk{
		{From: r22, To: lrt, Qualifier: domain.QualifierSame},
		{From: e2012, To: r22, Qualifier: domain.QualifierNarrower},
	} {
		if err := tx.UpsertCrosswalk(c); err != nil {
			t.Fatalf("UpsertCrosswalk: %v", err)
		}
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "BRO-01A", Rank: "alliance", Name: "Bromion erecti", Author: "Rivas-Martínez 1978"}); err != nil {
		t.Fatalf("UpsertSyntaxon: %v", err)
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "UNLINKED", Rank: "order", Name: "Nothing links here"}); err != nil {
		t.Fatalf("UpsertSyntaxon(unlinked): %v", err)
	}
	if err := tx.LinkSyntaxon(r22, "BRO-01A"); err != nil {
		t.Fatalf("LinkSyntaxon: %v", err)
	}
	for _, r := range []domain.SpeciesRole{
		{Key: r22, ConceptID: &concept, VerbatimName: "Bromus erectus", Role: "diagnostic", Fidelity: &fidelity, Provenance: "observed"},
		{Key: r22, VerbatimName: "Unresolvable dubia", Role: "constant", Provenance: "observed"},
		{Key: r99, ConceptID: &concept, VerbatimName: "Bromus erectus", Role: "dominant", Provenance: "observed"},
	} {
		if err := tx.UpsertSpeciesRole(r); err != nil {
			t.Fatalf("UpsertSpeciesRole(%q): %v", r.VerbatimName, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func openSeededDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)
	seedReadFixture(t, db)
	return db
}

func TestTypology_RoundTripAndNotFound(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.Typology(t.Context(), "eunis@2021")
	if err != nil {
		t.Fatalf("Typology: %v", err)
	}
	if got.Scheme != "eunis" || got.Version != "2021" {
		t.Errorf("Typology = %+v, want scheme eunis and version 2021", got)
	}

	if _, err := db.Typology(t.Context(), "bogus@1"); !errors.Is(err, output.ErrNotFound) {
		t.Errorf("Typology(bogus@1) error = %v, want it to wrap output.ErrNotFound", err)
	}
}

// TestTypologies_CountsHabitatTypesAndSortsByID exercises the LEFT JOIN
// count: eunis@2021 carries two seeded habitat types (r22, r99), annex1
// carries one (lrt). A third, freshly registered typology with no habitat
// types must still appear, with a measured 0 — not be silently dropped by
// the join.
func TestTypologies_CountsHabitatTypesAndSortsByID(t *testing.T) {
	db := openSeededDB(t)
	ctx := t.Context()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertTypology(domain.Typology{ID: "eunis@2012", Scheme: "eunis", Version: "2012"}); err != nil {
		t.Fatalf("UpsertTypology: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.Typologies(ctx)
	if err != nil {
		t.Fatalf("Typologies: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Typologies = %+v, want 3 entries", got)
	}
	// Sorted by id: annex1 < eunis@2012 < eunis@2021.
	wantIDs := []domain.TypologyID{"annex1", "eunis@2012", "eunis@2021"}
	for i, want := range wantIDs {
		if got[i].Typology.ID != want {
			t.Errorf("Typologies[%d].ID = %s, want %s", i, got[i].Typology.ID, want)
		}
	}
	counts := map[domain.TypologyID]int{}
	for _, s := range got {
		counts[s.Typology.ID] = s.HabitatTypes
	}
	if counts["eunis@2021"] != 2 {
		t.Errorf("eunis@2021 habitat_types = %d, want 2", counts["eunis@2021"])
	}
	if counts["annex1"] != 1 {
		t.Errorf("annex1 habitat_types = %d, want 1", counts["annex1"])
	}
	if counts["eunis@2012"] != 0 {
		t.Errorf("eunis@2012 habitat_types = %d, want 0 (registered but empty)", counts["eunis@2012"])
	}
}

func TestTypologies_EmptyIndexReturnsEmptySliceNotNil(t *testing.T) {
	db := openTestDB(t)

	got, err := db.Typologies(t.Context())
	if err != nil {
		t.Fatalf("Typologies: %v", err)
	}
	if got == nil {
		t.Error("Typologies() = nil, want an empty, non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("Typologies() = %+v, want none on a fresh index", got)
	}
}

// A crosswalk row is stored once but must be answerable from both ends —
// otherwise the Annex I entry direction of the API would see nothing.
func TestCrosswalks_FindsRowsInBothDirections(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.Crosswalks(t.Context(), r22)
	if err != nil {
		t.Fatalf("Crosswalks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Crosswalks(R22) = %+v, want both the outgoing and the incoming row", got)
	}

	fromAnnex, err := db.Crosswalks(t.Context(), lrt)
	if err != nil {
		t.Fatalf("Crosswalks(annex1): %v", err)
	}
	if len(fromAnnex) != 1 || fromAnnex[0].From != r22 {
		t.Errorf("Crosswalks(annex1:6510) = %+v, want the single row pointing at it", fromAnnex)
	}
}

func TestCrosswalks_EmptyIsAnEmptySliceNotNil(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.Crosswalks(t.Context(), r99)
	if err != nil {
		t.Fatalf("Crosswalks: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Crosswalks(R99) = %+v, want an empty, non-nil slice", got)
	}
}

func TestSpeciesRoles_FilterAndNullables(t *testing.T) {
	db := openSeededDB(t)

	all, err := db.SpeciesRoles(t.Context(), r22, "")
	if err != nil {
		t.Fatalf("SpeciesRoles(all): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("SpeciesRoles(R22, all) = %+v, want both species", all)
	}

	diagnostic, err := db.SpeciesRoles(t.Context(), r22, "diagnostic")
	if err != nil {
		t.Fatalf("SpeciesRoles(diagnostic): %v", err)
	}
	if len(diagnostic) != 1 || diagnostic[0].VerbatimName != "Bromus erectus" {
		t.Fatalf("SpeciesRoles(R22, diagnostic) = %+v, want only Bromus erectus", diagnostic)
	}
	if diagnostic[0].ConceptID == nil || *diagnostic[0].ConceptID != "wcvp-1" {
		t.Errorf("ConceptID = %v, want wcvp-1", diagnostic[0].ConceptID)
	}
	if diagnostic[0].Fidelity == nil || *diagnostic[0].Fidelity != 49.6 {
		t.Errorf("Fidelity = %v, want 49.6", diagnostic[0].Fidelity)
	}
	if diagnostic[0].Constancy != nil {
		t.Errorf("Constancy = %v, want nil — a missing value must not become 0", *diagnostic[0].Constancy)
	}

	constant, err := db.SpeciesRoles(t.Context(), r22, "constant")
	if err != nil {
		t.Fatalf("SpeciesRoles(constant): %v", err)
	}
	if len(constant) != 1 || constant[0].ConceptID != nil {
		t.Errorf("SpeciesRoles(R22, constant) = %+v, want the unresolved name with a nil ConceptID", constant)
	}
}

// Constancy must round-trip too, the same way Fidelity already does.
func TestSpeciesRoles_RoundTripsConstancy(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	constancy := 72.5

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSpeciesRole(domain.SpeciesRole{
		Key: key, VerbatimName: "Bromus erectus", Role: "constant", Constancy: &constancy,
	}); err != nil {
		t.Fatalf("UpsertSpeciesRole: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SpeciesRoles(ctx, key, "")
	if err != nil {
		t.Fatalf("SpeciesRoles: %v", err)
	}
	if len(got) != 1 || got[0].Constancy == nil || *got[0].Constancy != constancy {
		t.Errorf("SpeciesRoles = %+v, want Constancy = %v", got, constancy)
	}
}

func TestSpeciesRolesByConcept_ReturnsEveryRoleAndKey(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.SpeciesRolesByConcept(t.Context(), "wcvp-1")
	if err != nil {
		t.Fatalf("SpeciesRolesByConcept: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("SpeciesRolesByConcept = %+v, want the roles in both habitat types", got)
	}
	seen := map[string]string{}
	for _, r := range got {
		seen[r.Key.String()] = r.Role
	}
	if seen[r22.String()] != "diagnostic" || seen[r99.String()] != "dominant" {
		t.Errorf("roles by key = %v, want R22 diagnostic and R99 dominant", seen)
	}

	unknown, err := db.SpeciesRolesByConcept(t.Context(), "wcvp-nope")
	if err != nil {
		t.Fatalf("SpeciesRolesByConcept(unknown): %v", err)
	}
	if len(unknown) != 0 {
		t.Errorf("SpeciesRolesByConcept(unknown) = %+v, want nothing", unknown)
	}
}

// SpeciesRolesByConcept must round-trip DerivedFrom too, the same way
// SpeciesRoles already does.
func TestSpeciesRolesByConcept_ReturnsDerivedFromWhenSet(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	member := "wcvp:concept:100"
	aggregate := "wcvp:concept:99"

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSpeciesRole(domain.SpeciesRole{
		Key: key, ConceptID: &member, VerbatimName: "Rubus caesius", Role: "diagnostic",
		Provenance: "derived_from_aggregate", DerivedFrom: &aggregate,
	}); err != nil {
		t.Fatalf("UpsertSpeciesRole: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SpeciesRolesByConcept(ctx, member)
	if err != nil {
		t.Fatalf("SpeciesRolesByConcept: %v", err)
	}
	if len(got) != 1 || got[0].DerivedFrom == nil || *got[0].DerivedFrom != aggregate {
		t.Errorf("SpeciesRolesByConcept = %+v, want one row with DerivedFrom = %q", got, aggregate)
	}
}

func TestSyntaxonAndSyntaxa(t *testing.T) {
	db := openSeededDB(t)

	s, err := db.Syntaxon(t.Context(), "BRO-01A")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if s.Rank != "alliance" || s.Name != "Bromion erecti" {
		t.Errorf("Syntaxon = %+v, want the seeded alliance", s)
	}
	if s.Author != "Rivas-Martínez 1978" {
		t.Errorf("Author = %q, want %q", s.Author, "Rivas-Martínez 1978")
	}
	if _, err := db.Syntaxon(t.Context(), "NOPE"); !errors.Is(err, output.ErrNotFound) {
		t.Errorf("Syntaxon(NOPE) error = %v, want it to wrap output.ErrNotFound", err)
	}

	linked, err := db.Syntaxa(t.Context(), r22)
	if err != nil {
		t.Fatalf("Syntaxa: %v", err)
	}
	if len(linked) != 1 || linked[0].ID != "BRO-01A" {
		t.Errorf("Syntaxa(R22) = %+v, want the linked alliance", linked)
	}
	none, err := db.Syntaxa(t.Context(), r99)
	if err != nil {
		t.Fatalf("Syntaxa(R99): %v", err)
	}
	if none == nil || len(none) != 0 {
		t.Errorf("Syntaxa(R99) = %+v, want an empty, non-nil slice", none)
	}
}

// seedReadFixture seeds two syntaxa (BRO-01A linked to R22, and UNLINKED
// linked to nothing), so AllSyntaxa must return both, in id order — it lists
// every vegetation unit the index holds, not just the linked ones.
func TestAllSyntaxa(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.AllSyntaxa(t.Context())
	if err != nil {
		t.Fatalf("AllSyntaxa: %v", err)
	}
	if len(got) != 2 || got[0].ID != "BRO-01A" || got[1].ID != "UNLINKED" {
		t.Errorf("AllSyntaxa = %+v, want [BRO-01A, UNLINKED] in id order", got)
	}
}

// A syntaxon that exists but is linked to nothing must be distinguishable from
// one that does not exist — hence Syntaxon() next to the key lookup.
func TestHabitatTypeKeysForSyntaxon(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.HabitatTypeKeysForSyntaxon(t.Context(), "BRO-01A")
	if err != nil {
		t.Fatalf("HabitatTypeKeysForSyntaxon: %v", err)
	}
	if len(got) != 1 || got[0] != r22 {
		t.Errorf("HabitatTypeKeysForSyntaxon(BRO-01A) = %+v, want [%s]", got, r22)
	}

	unlinked, err := db.HabitatTypeKeysForSyntaxon(t.Context(), "UNLINKED")
	if err != nil {
		t.Fatalf("HabitatTypeKeysForSyntaxon(UNLINKED): %v", err)
	}
	if len(unlinked) != 0 {
		t.Errorf("HabitatTypeKeysForSyntaxon(UNLINKED) = %+v, want nothing", unlinked)
	}
}

// Every read must surface a query failure instead of an empty answer.
func TestReads_QueryErrorsAreReturned(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ctx := context.Background()

	cases := map[string]func() error{
		"Typology":                    func() error { _, err := db.Typology(ctx, "eunis@2021"); return err },
		"Crosswalks":                  func() error { _, err := db.Crosswalks(ctx, r22); return err },
		"SpeciesRoles":                func() error { _, err := db.SpeciesRoles(ctx, r22, ""); return err },
		"SpeciesRolesByConcept":       func() error { _, err := db.SpeciesRolesByConcept(ctx, "wcvp-1"); return err },
		"Syntaxon":                    func() error { _, err := db.Syntaxon(ctx, "BRO-01A"); return err },
		"SyntaxonByEEACode":           func() error { _, err := db.SyntaxonByEEACode(ctx, "PAP-01A"); return err },
		"Syntaxa":                     func() error { _, err := db.Syntaxa(ctx, r22); return err },
		"HabitatTypeKeysForSyntaxon":  func() error { _, err := db.HabitatTypeKeysForSyntaxon(ctx, "BRO-01A"); return err },
		"SyntaxonChildren":            func() error { _, err := db.SyntaxonChildren(ctx, "CA01"); return err },
		"HabitatTypeCountForSyntaxon": func() error { _, err := db.HabitatTypeCountForSyntaxon(ctx, "CA01A"); return err },
		"AreasForConcepts": func() error {
			_, err := db.AreasForConcepts(ctx, []string{"wcvp:concept:1"}, domain.SchemeWGSRPDL3)
			return err
		},
		"KnownAreaCodes": func() error { _, err := db.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3); return err },
		"KnownAreaCodesEVCTerritory": func() error {
			_, err := db.KnownAreaCodes(ctx, domain.SchemeEVCTerritory)
			return err
		},
		"SyntaxonDistribution": func() error {
			_, err := db.SyntaxonDistribution(ctx, "CA01A", domain.SchemeEVCTerritory)
			return err
		},
		"SyntaxonOccurrencesInArea": func() error {
			_, err := db.SyntaxonOccurrencesInArea(ctx, domain.SchemeEVCTerritory, "albania")
			return err
		},
		"SyntaxaWithCoverage": func() error {
			_, err := db.SyntaxaWithCoverage(ctx, domain.SchemeEVCTerritory)
			return err
		},
		"AllSyntaxa":    func() error { _, err := db.AllSyntaxa(ctx); return err },
		"SyntaxaByRank": func() error { _, err := db.SyntaxaByRank(ctx, "alliance", ""); return err },
		"SyntaxaByRankWithGroup": func() error {
			_, err := db.SyntaxaByRank(ctx, "alliance", domain.LifeFormPhanerogam)
			return err
		},
		"SyntaxonRanks": func() error { _, err := db.SyntaxonRanks(ctx); return err },
	}
	for name, call := range cases {
		if err := call(); err == nil {
			t.Errorf("%s on a closed database = nil error, want an error", name)
		} else if !strings.HasPrefix(err.Error(), "sqlite: ") {
			t.Errorf("%s error = %q, want the adapter's own context prefixed", name, err)
		}
	}
}

// The rows.Err()/Scan paths of the list reads, exercised deterministically with
// the stub driver instead of racing a cancellation against row iteration.
func TestReads_RowsIterationAndScanErrorsAreReturned(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		call func(db *DB) error
		rows string
		scan string
	}{
		"Crosswalks": {
			call: func(db *DB) error { _, err := db.Crosswalks(ctx, r22); return err },
			rows: "reading crosswalks", scan: "scanning crosswalk",
		},
		"SpeciesRoles": {
			call: func(db *DB) error { _, err := db.SpeciesRoles(ctx, r22, ""); return err },
			rows: "reading species", scan: "scanning species",
		},
		"SpeciesRolesByConcept": {
			call: func(db *DB) error { _, err := db.SpeciesRolesByConcept(ctx, "wcvp-1"); return err },
			rows: "reading habitat types of concept", scan: "scanning habitat types of concept",
		},
		"AreasForConcepts": {
			call: func(db *DB) error {
				_, err := db.AreasForConcepts(ctx, []string{"wcvp:concept:1"}, domain.SchemeWGSRPDL3)
				return err
			},
			rows: "iterating distribution", scan: "scanning distribution",
		},
		"KnownAreaCodes": {
			call: func(db *DB) error { _, err := db.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3); return err },
			rows: "iterating area codes", scan: "scanning area code",
		},
		// The evc_territory branch issues its own statement (over
		// syntaxon_distribution, not species_distribution), so it needs its
		// own error-path entry: the wgsrpd_l3 case above does not exercise it.
		"KnownAreaCodesEVCTerritory": {
			call: func(db *DB) error { _, err := db.KnownAreaCodes(ctx, domain.SchemeEVCTerritory); return err },
			rows: "iterating syntaxon area codes", scan: "scanning syntaxon area code",
		},
		"SyntaxonOccurrencesInArea": {
			call: func(db *DB) error {
				_, err := db.SyntaxonOccurrencesInArea(ctx, domain.SchemeEVCTerritory, "albania")
				return err
			},
			rows: "iterating occurrences in area", scan: "scanning occurrence",
		},
		"SyntaxaWithCoverage": {
			call: func(db *DB) error { _, err := db.SyntaxaWithCoverage(ctx, domain.SchemeEVCTerritory); return err },
			rows: "iterating syntaxa with distribution coverage", scan: "scanning syntaxa with distribution coverage",
		},
		// SyntaxonDistribution's coverage check is a single QueryRowContext,
		// answered first — under the plain rows/scan modes it is what fails,
		// before the distribution-rows query is ever reached. The distinct
		// second-query error path is exercised separately below with its own
		// stub modes, since the two queries are not distinguishable by
		// query text alone under a mode that fails every query.
		"SyntaxonDistribution_Coverage": {
			call: func(db *DB) error {
				_, err := db.SyntaxonDistribution(ctx, "CA01A", domain.SchemeEVCTerritory)
				return err
			},
			rows: "reading syntaxon coverage", scan: "reading syntaxon coverage",
		},
		"Syntaxa": {
			call: func(db *DB) error { _, err := db.Syntaxa(ctx, r22); return err },
			rows: "reading syntaxa", scan: "scanning syntaxa",
		},
		"HabitatTypeKeysForSyntaxon": {
			call: func(db *DB) error { _, err := db.HabitatTypeKeysForSyntaxon(ctx, "BRO-01A"); return err },
			rows: "reading habitat types of syntaxon", scan: "scanning habitat types of syntaxon",
		},
		// SyntaxonChildren wraps both scanSyntaxa failure shapes (the Scan
		// error inside the loop and the rows.Err() after it) into the same
		// message, unlike the other reads here that word them differently —
		// scanSyntaxa has exactly one error return, on purpose.
		"SyntaxonChildren": {
			call: func(db *DB) error { _, err := db.SyntaxonChildren(ctx, "CA01"); return err },
			rows: "reading children of syntaxon", scan: "reading children of syntaxon",
		},
		"AllSyntaxa": {
			call: func(db *DB) error { _, err := db.AllSyntaxa(ctx); return err },
			rows: "reading all syntaxa", scan: "scanning syntaxon",
		},
		// SyntaxonByEEACode reads rows rather than a single row since it has to
		// tell one match from several, so it has these two paths at all.
		"SyntaxonByEEACode": {
			call: func(db *DB) error { _, err := db.SyntaxonByEEACode(ctx, "PAP-01A"); return err },
			rows: "reading syntaxon by eea_code", scan: "reading syntaxon by eea_code",
		},
		"SyntaxaByRank": {
			call: func(db *DB) error { _, err := db.SyntaxaByRank(ctx, "alliance", ""); return err },
			rows: "reading syntaxa of rank", scan: "reading syntaxa of rank",
		},
		"SyntaxaByRankWithGroup": {
			call: func(db *DB) error {
				_, err := db.SyntaxaByRank(ctx, "alliance", domain.LifeFormPhanerogam)
				return err
			},
			rows: "reading syntaxa of rank", scan: "reading syntaxa of rank",
		},
		"SyntaxonRanks": {
			call: func(db *DB) error { _, err := db.SyntaxonRanks(ctx); return err },
			rows: "reading syntaxon ranks", scan: "scanning syntaxon rank",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for mode, want := range map[stubMode]string{stubModeRowsErr: tc.rows, stubModeScanErr: tc.scan} {
				err := tc.call(&DB{DB: newStubDB(t, mode)})
				if err == nil {
					t.Fatalf("%s in mode %v = nil error, want an error", name, mode)
				}
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%s error = %q, want it to name %q", name, err, want)
				}
			}
		})
	}
}

// SyntaxonDistribution's second query (the occurrence rows) is unreachable
// through the plain stubModeRowsErr/stubModeScanErr modes: those fail every
// query, so the coverage check above it always fails first. The two custom
// modes below let the coverage query answer cleanly (no row -> ErrNoRows,
// Covered stays false) and fail only the distribution-rows query that
// follows — the stubModeFirstSyntaxonLookupThenFails pattern, applied here
// because the two queries are not distinguishable by query text alone under
// a mode that fails indiscriminately.
func TestSyntaxonDistribution_DistributionRowsIterationAndScanErrorsAreReturned(t *testing.T) {
	ctx := context.Background()
	for mode, want := range map[stubMode]string{
		stubModeSyntaxonDistributionRowsErr: "iterating syntaxon distribution",
		stubModeSyntaxonDistributionScanErr: "scanning syntaxon distribution",
	} {
		_, err := (&DB{DB: newStubDB(t, mode)}).SyntaxonDistribution(ctx, "CA01A", domain.SchemeEVCTerritory)
		if err == nil {
			t.Fatalf("SyntaxonDistribution in mode %v = nil error, want an error", mode)
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name %q", err, want)
		}
	}
}

// A concept with no rows must be ABSENT from the map, not present-and-empty:
// the read side turns absence into "unknown" and an empty list would become
// "does not occur here", which is a different and wrong statement.
func TestAreasForConcepts_ConceptWithoutDataIsAbsent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertDistribution("wcvp:concept:1", domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}); err != nil {
		t.Fatalf("UpsertDistribution: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.AreasForConcepts(ctx, []string{"wcvp:concept:1", "wcvp:concept:2"}, domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("AreasForConcepts: %v", err)
	}
	if _, ok := got["wcvp:concept:2"]; ok {
		t.Error("a concept without distribution rows must be absent from the map, not empty-valued")
	}
}

func TestAreasForConcepts_EmptyInputNeedsNoQuery(t *testing.T) {
	db := openTestDB(t)
	got, err := db.AreasForConcepts(context.Background(), nil, domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("AreasForConcepts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

// SQLite caps bound parameters (SQLITE_LIMIT_VARIABLE_NUMBER, measured at 32766
// with this driver), and the query builds one placeholder per concept id. A
// caller with more ids than that must still get an answer instead of a "too many
// SQL variables" error, so the call is chunked — and this asserts it across more
// than one chunk boundary as well as past the driver's own ceiling.
func TestAreasForConcepts_ChunksPastTheBoundParameterLimit(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// One seeded concept in the first chunk and one far past the driver's limit,
	// so a chunking bug that drops all but the first chunk cannot pass.
	seeded := map[string]string{"wcvp:concept:1": "GER", "wcvp:concept:33000": "FRA"}
	for id, code := range seeded {
		if err := tx.UpsertDistribution(id, domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: code}); err != nil {
			t.Fatalf("UpsertDistribution(%s): %v", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	ids := make([]string, 0, 33001)
	for i := range 33001 {
		ids = append(ids, fmt.Sprintf("wcvp:concept:%d", i))
	}

	got, err := db.AreasForConcepts(ctx, ids, domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("AreasForConcepts with %d ids: %v", len(ids), err)
	}
	for id, code := range seeded {
		if !slices.Equal(got[id], []string{code}) {
			t.Errorf("areas[%s] = %v, want %v — a chunk was lost", id, got[id], []string{code})
		}
	}
	if len(got) != len(seeded) {
		t.Errorf("got %d concepts with data, want %d", len(got), len(seeded))
	}
}

func TestKnownAreaCodes_ListsWhatTheIndexHas(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, code := range []string{"GER", "FRA", "GER"} {
		if err := tx.UpsertDistribution("wcvp:concept:1", domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: code}); err != nil {
			t.Fatalf("UpsertDistribution %s: %v", code, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("KnownAreaCodes: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("codes = %v, want two distinct codes", got)
	}
}

func TestConceptIDs_DistinctAndWithoutNulls(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	id := "wcvp:concept:1"

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, r := range []domain.SpeciesRole{
		{Key: key, ConceptID: &id, VerbatimName: "A", Role: "diagnostic"},
		{Key: key, ConceptID: &id, VerbatimName: "A2", Role: "constant"},
		{Key: key, ConceptID: nil, VerbatimName: "Moss", Role: "diagnostic"},
	} {
		if err := tx.UpsertSpeciesRole(r); err != nil {
			t.Fatalf("UpsertSpeciesRole: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.ConceptIDs(ctx)
	if err != nil {
		t.Fatalf("ConceptIDs: %v", err)
	}
	if len(got) != 1 || got[0] != id {
		t.Errorf("ConceptIDs() = %v, want exactly [%s]", got, id)
	}
}
