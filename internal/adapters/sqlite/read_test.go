package sqlite

import (
	"context"
	"errors"
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
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "BRO-01A", Rank: "alliance", Name: "Bromion erecti"}); err != nil {
		t.Fatalf("UpsertSyntaxon: %v", err)
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "UNLINKED", Rank: "order", Name: "Nothing links here"}); err != nil {
		t.Fatalf("UpsertSyntaxon(unlinked): %v", err)
	}
	if err := tx.LinkSyntaxon(r22, "BRO-01A"); err != nil {
		t.Fatalf("LinkSyntaxon: %v", err)
	}
	for _, r := range []domain.SpeciesRole{
		{Key: r22, ConceptID: &concept, VerbatimName: "Bromus erectus", Role: "diagnostic", Fidelity: &fidelity},
		{Key: r22, VerbatimName: "Unresolvable dubia", Role: "constant"},
		{Key: r99, ConceptID: &concept, VerbatimName: "Bromus erectus", Role: "dominant"},
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

func TestSyntaxonAndSyntaxa(t *testing.T) {
	db := openSeededDB(t)

	s, err := db.Syntaxon(t.Context(), "BRO-01A")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if s.Rank != "alliance" || s.Name != "Bromion erecti" {
		t.Errorf("Syntaxon = %+v, want the seeded alliance", s)
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
		"Typology":                   func() error { _, err := db.Typology(ctx, "eunis@2021"); return err },
		"Crosswalks":                 func() error { _, err := db.Crosswalks(ctx, r22); return err },
		"SpeciesRoles":               func() error { _, err := db.SpeciesRoles(ctx, r22, ""); return err },
		"SpeciesRolesByConcept":      func() error { _, err := db.SpeciesRolesByConcept(ctx, "wcvp-1"); return err },
		"Syntaxon":                   func() error { _, err := db.Syntaxon(ctx, "BRO-01A"); return err },
		"Syntaxa":                    func() error { _, err := db.Syntaxa(ctx, r22); return err },
		"HabitatTypeKeysForSyntaxon": func() error { _, err := db.HabitatTypeKeysForSyntaxon(ctx, "BRO-01A"); return err },
		"AreasForConcepts": func() error {
			_, err := db.AreasForConcepts(ctx, []string{"wcvp:concept:1"}, domain.SchemeWGSRPDL3)
			return err
		},
		"KnownAreaCodes": func() error { _, err := db.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3); return err },
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
		"Syntaxa": {
			call: func(db *DB) error { _, err := db.Syntaxa(ctx, r22); return err },
			rows: "reading syntaxa", scan: "scanning syntaxa",
		},
		"HabitatTypeKeysForSyntaxon": {
			call: func(db *DB) error { _, err := db.HabitatTypeKeysForSyntaxon(ctx, "BRO-01A"); return err },
			rows: "reading habitat types of syntaxon", scan: "scanning habitat types of syntaxon",
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
