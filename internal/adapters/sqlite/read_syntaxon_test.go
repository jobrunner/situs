package sqlite

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

func TestSyntaxonIDsByAltCode(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "AA01A", Rank: domain.SyntaxonRankAlliance,
		Name: "X", AltCode: "PAP-01A", Source: domain.SyntaxonSourceEVC,
		ParentProvenance: domain.ParentProvenanceOfficial}); err != nil {
		t.Fatalf("UpsertSyntaxon(AA01A): %v", err)
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "AA01B", Rank: domain.SyntaxonRankAlliance,
		Name: "Y", Source: domain.SyntaxonSourceEVC,
		ParentProvenance: domain.ParentProvenanceOfficial}); err != nil {
		t.Fatalf("UpsertSyntaxon(AA01B): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := db.SyntaxonIDsByAltCode(ctx)
	if err != nil {
		t.Fatalf("SyntaxonIDsByAltCode: %v", err)
	}
	if len(got) != 1 || got["PAP-01A"] != "AA01A" {
		t.Errorf("Karte = %v, erwartet genau PAP-01A->AA01A", got)
	}
}

// seedSyntaxaHierarchy fills the small world every navigation test asks about:
// two complete chains formation -> class -> order -> alliance, one phanerogam
// and one cryptogam, plus two alliances under the same order so the ordering
// promise is checkable.
func seedSyntaxaHierarchy(t *testing.T, db *DB) {
	t.Helper()
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	// Deliberately NOT inserted in id order: the reads' ordering promise must
	// not come from the insertion order.
	for _, s := range []domain.Syntaxon{
		{ID: "CA01B", Rank: domain.SyntaxonRankAlliance, Name: "Zweiter Verband",
			Author: "Moor 1975", ParentID: "CA01", AltCode: "TST-01B",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01A", Rank: domain.SyntaxonRankAlliance, Name: "Erster Verband",
			Author: "Moor 1970", ParentID: "CA01", AltCode: "TST-01A",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01", Rank: domain.SyntaxonRankOrder, Name: "Testordnung",
			Author: "Moor 1960", ParentID: "CA", AltCode: "TST-01",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA", Rank: domain.SyntaxonRankClass, Name: "Testklasse",
			Author: "Moor 1950", ParentID: "C", AltCode: "TST",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "C", Rank: domain.SyntaxonRankFormation, Name: "Vegetation of the nemoral forest zone",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial,
			LifeFormGroup: domain.LifeFormPhanerogam},
		{ID: "RA01A", Rank: domain.SyntaxonRankAlliance, Name: "Moosverband",
			ParentID: "RA01", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA01", Rank: domain.SyntaxonRankOrder, Name: "Moosordnung",
			ParentID: "RA", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA", Rank: domain.SyntaxonRankClass, Name: "Moosklasse",
			ParentID: "R", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "R", Rank: domain.SyntaxonRankFormation, Name: "Epigaeic bryophyte and lichen vegetation",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial,
			LifeFormGroup: domain.LifeFormBryophyteLichen},
	} {
		if err := tx.UpsertSyntaxon(s); err != nil {
			t.Fatalf("UpsertSyntaxon(%s): %v", s.ID, err)
		}
	}
	for _, ty := range []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}} {
		if err := tx.UpsertTypology(ty); err != nil {
			t.Fatalf("UpsertTypology: %v", err)
		}
	}
	for _, h := range []domain.HabitatType{
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T11"}, NameEN: "Wald"},
		{Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T12"}, NameEN: "Anderer Wald"},
	} {
		if err := tx.UpsertHabitatType(h); err != nil {
			t.Fatalf("UpsertHabitatType(%s): %v", h.Key, err)
		}
	}
	for _, code := range []string{"T11", "T12"} {
		if err := tx.LinkSyntaxon(domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}, "CA01A"); err != nil {
			t.Fatalf("LinkSyntaxon(%s): %v", code, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func openHierarchyDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)
	seedSyntaxaHierarchy(t, db)
	return db
}

func syntaxonIDs(in []domain.Syntaxon) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, s.ID)
	}
	return out
}

func TestSyntaxonChildrenSindNachIDSortiert(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonChildren(t.Context(), "CA01")
	if err != nil {
		t.Fatalf("SyntaxonChildren: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"CA01A", "CA01B"}) {
		t.Errorf("Kinder = %v, erwartet [CA01A CA01B]", syntaxonIDs(got))
	}
	// Subproject A's extra columns have to travel along, otherwise the API
	// withholds what the index knows.
	if got[0].AltCode != "TST-01A" || got[0].Source != domain.SyntaxonSourceEVC ||
		got[0].ParentProvenance != domain.ParentProvenanceOfficial {
		t.Errorf("CA01A = %+v, erwartet Altcode, Quelle und Elternteil-Provenienz", got[0])
	}
}

func TestSyntaxonChildrenEinesVerbandsSindLeerUndNichtNil(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonChildren(t.Context(), "CA01A")
	if err != nil {
		t.Fatalf("SyntaxonChildren: %v", err)
	}
	if got == nil {
		t.Fatal("Kinder = nil; eine leere Liste ist die Antwort, nil wuerde als JSON-null durchschlagen")
	}
	if len(got) != 0 {
		t.Errorf("Kinder = %v, erwartet leer", syntaxonIDs(got))
	}
}

func TestHabitatTypeCountForSyntaxonZaehltNurDieEigenenKanten(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.HabitatTypeCountForSyntaxon(t.Context(), "CA01A")
	if err != nil {
		t.Fatalf("HabitatTypeCountForSyntaxon: %v", err)
	}
	if got != 2 {
		t.Errorf("Kantenzahl von CA01A = %d, erwartet 2", got)
	}
	// The order above it carries no edge of its own. 0 is the right answer and
	// not an error — which is why the field is called
	// direct_habitat_type_count.
	parent, err := db.HabitatTypeCountForSyntaxon(t.Context(), "CA01")
	if err != nil {
		t.Fatalf("HabitatTypeCountForSyntaxon(CA01): %v", err)
	}
	if parent != 0 {
		t.Errorf("Kantenzahl von CA01 = %d, erwartet 0", parent)
	}
}

func TestSyntaxonAncestorsAeussersteZuerst(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonAncestors(t.Context(), "CA01A")
	if err != nil {
		t.Fatalf("SyntaxonAncestors: %v", err)
	}
	// Formation, class, order — in that order, so a client can print the path
	// unchanged as a breadcrumb trail.
	if !slices.Equal(syntaxonIDs(got), []string{"C", "CA", "CA01"}) {
		t.Errorf("Ahnen = %v, erwartet [C CA CA01]", syntaxonIDs(got))
	}
	if got[0].LifeFormGroup != domain.LifeFormPhanerogam {
		t.Errorf("Formation im Pfad = %+v, erwartet life_form_group phanerogam", got[0])
	}
}

func TestSyntaxonAncestorsEinerFormationSindLeerUndNichtNil(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonAncestors(t.Context(), "C")
	if err != nil {
		t.Fatalf("SyntaxonAncestors: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Ahnen einer Formation = %v, erwartet eine leere, nicht-nil Liste", got)
	}
}

func TestSyntaxonAncestorsUnbekannteIDIstNotFound(t *testing.T) {
	db := openHierarchyDB(t)

	_, err := db.SyntaxonAncestors(t.Context(), "GIBTSNICHT")
	if !errors.Is(err, output.ErrNotFound) {
		t.Errorf("Fehler = %v, erwartet output.ErrNotFound", err)
	}
}

func TestSyntaxonAncestorsMeldetBaumelndeElternreferenz(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSyntaxon(domain.Syntaxon{ID: "WAISE", Rank: domain.SyntaxonRankAlliance,
		Name: "Verband mit ins Leere zeigendem Elternteil", ParentID: "FEHLT",
		Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived}); err != nil {
		t.Fatalf("UpsertSyntaxon: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	_, err = db.SyntaxonAncestors(t.Context(), "WAISE")
	if err == nil {
		t.Fatal("SyntaxonAncestors hat eine baumelnde parent_id ueberbrueckt")
	}
	if !strings.Contains(err.Error(), "FEHLT") {
		t.Errorf("Fehler benennt das fehlende Elternteil nicht: %v", err)
	}
	// The decisive part: this is an index defect (500), not a missing result
	// (404). Were the error to wrap output.ErrNotFound, the use case would
	// turn it into a NOT_FOUND for an id that does exist.
	if errors.Is(err, output.ErrNotFound) {
		t.Error("der Fehler umwickelt output.ErrNotFound; eine baumelnde Referenz darf kein 404 werden")
	}
}

func TestSyntaxonAncestorsLaeuftBeiEinemZykelInDenTiefenfehler(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, s := range []domain.Syntaxon{
		{ID: "ZYK-A", Rank: domain.SyntaxonRankAlliance, Name: "A", ParentID: "ZYK-B",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
		{ID: "ZYK-B", Rank: domain.SyntaxonRankOrder, Name: "B", ParentID: "ZYK-A",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
	} {
		if err := tx.UpsertSyntaxon(s); err != nil {
			t.Fatalf("UpsertSyntaxon(%s): %v", s.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	_, err = db.SyntaxonAncestors(t.Context(), "ZYK-A")
	if err == nil {
		t.Fatal("SyntaxonAncestors lief in einem Zykel durch")
	}
	if !strings.Contains(err.Error(), "ZYK-A") {
		t.Errorf("Fehler benennt die ausloesende ID nicht: %v", err)
	}
}

// A query failure partway up the walk (the outer lookup succeeds, a later one
// does not) is a third failure mode distinct from a dangling parent_id: it is
// wrapped as a plain error too, but with the underlying error message instead
// of a "no row carries it" claim the index cannot back up. The stub driver
// makes this deterministic — a real database has no reliable way to make the
// second of two lookups fail while the first succeeds.
func TestSyntaxonAncestorsMeldetFehlerBeimWeiterenAufstieg(t *testing.T) {
	db := &DB{DB: newStubDB(t, stubModeFirstSyntaxonLookupThenFails)}

	_, err := db.SyntaxonAncestors(t.Context(), "START")
	if err == nil {
		t.Fatal("SyntaxonAncestors bei einem Fehler weiter oben im Aufstieg = nil error")
	}
	if !strings.Contains(err.Error(), "walking the ancestors") {
		t.Errorf("Fehler = %q, erwartet 'walking the ancestors'", err)
	}
	if errors.Is(err, output.ErrNotFound) {
		t.Error("der Fehler umwickelt output.ErrNotFound; ein Abfragefehler beim Aufstieg ist kein 404")
	}
}

func TestSyntaxaByRankOhneGruppenfilter(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankClass, "")
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"CA", "RA"}) {
		t.Errorf("Klassen = %v, erwartet [CA RA]", syntaxonIDs(got))
	}
}

func TestSyntaxaByRankFiltertVerbaendeUeberDreiEbenenNachOben(t *testing.T) {
	db := openHierarchyDB(t)

	// Per subproject A the filter value sits on the formation row ONLY. An
	// alliance is three levels away — which is what this test exercises.
	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormBryophyteLichen)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"RA01A"}) {
		t.Errorf("Moosverbaende = %v, erwartet [RA01A]", syntaxonIDs(got))
	}

	phanerogam, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormPhanerogam)
	if err != nil {
		t.Fatalf("SyntaxaByRank(phanerogam): %v", err)
	}
	if !slices.Equal(syntaxonIDs(phanerogam), []string{"CA01A", "CA01B"}) {
		t.Errorf("phanerogame Verbaende = %v, erwartet [CA01A CA01B]", syntaxonIDs(phanerogam))
	}
}

func TestSyntaxaByRankFiltertAuchAufDerFormationsebene(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankFormation, domain.LifeFormBryophyteLichen)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(syntaxonIDs(got), []string{"R"}) {
		t.Errorf("Formationen = %v, erwartet [R]", syntaxonIDs(got))
	}
}

func TestSyntaxaByRankUnbekannterRangIstEineLeereListe(t *testing.T) {
	db := openHierarchyDB(t)

	// Validating the value is the read side's job, not the repository's: here
	// "there are none" is the honest answer; the use case turns it into
	// INVALID_QUERY (task 3).
	got, err := db.SyntaxaByRank(t.Context(), "association", "")
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Ergebnis = %v, erwartet eine leere, nicht-nil Liste", got)
	}
}

func TestSyntaxaByRankLaeuftBeiEinemZykelNichtEndlos(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for _, s := range []domain.Syntaxon{
		{ID: "ZYK-A", Rank: domain.SyntaxonRankAlliance, Name: "A", ParentID: "ZYK-B",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
		{ID: "ZYK-B", Rank: domain.SyntaxonRankOrder, Name: "B", ParentID: "ZYK-A",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
	} {
		if err := tx.UpsertSyntaxon(s); err != nil {
			t.Fatalf("UpsertSyntaxon(%s): %v", s.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Without the step bound in the CTE this query does not fail — it never
	// returns at all. It has to terminate and yield an empty list: neither row
	// reaches a formation.
	got, err := db.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormPhanerogam)
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Ergebnis = %v, erwartet leer — keine Zeile erreicht eine Formation", syntaxonIDs(got))
	}
}

// TestSyntaxaByRankStepBoundMatchesMaxSyntaxonAncestors guards the seam a
// comment alone cannot: the recursive CTE's step bound is a literal, not a
// bound parameter (mandatory per the task brief — building it from Go input
// is exactly the SQL assembly gosec G201 forbids), so nothing at compile time
// ties it to maxSyntaxonAncestors. Without this test, raising
// maxSyntaxonAncestors for a fifth rank would silently leave the CTE cutting
// off one step early — SyntaxaByRank would then report the new deepest rank
// as always empty under any group filter, and every other test here (built
// on a three-step hierarchy) would stay green.
func TestSyntaxaByRankStepBoundMatchesMaxSyntaxonAncestors(t *testing.T) {
	want := fmt.Sprintf("u.steps < %d", maxSyntaxonAncestors)
	if !strings.Contains(syntaxaByRankRowsWithGroupSQL, want) {
		t.Errorf("syntaxaByRankRowsWithGroupSQL does not contain %q — the CTE step bound has drifted from maxSyntaxonAncestors (%d)",
			want, maxSyntaxonAncestors)
	}
}

// TestJedeZeileErreichtUeberDenAhnenpfadEineFormation is the design's central
// promise, checked at the read rather than at the table: every row reaches a
// formation through SyntaxonAncestors, and the path is outermost-first.
// Subproject A checks the same thing at ingest time; here it is checked where
// a user experiences it.
func TestJedeZeileErreichtUeberDenAhnenpfadEineFormation(t *testing.T) {
	db := openHierarchyDB(t)

	all, err := db.AllSyntaxa(t.Context())
	if err != nil {
		t.Fatalf("AllSyntaxa: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("die Fixture ist leer; der Test wuerde nichts pruefen")
	}
	for _, s := range all {
		ancestors, err := db.SyntaxonAncestors(t.Context(), s.ID)
		if err != nil {
			t.Fatalf("SyntaxonAncestors(%s): %v", s.ID, err)
		}
		if s.Rank == domain.SyntaxonRankFormation {
			if len(ancestors) != 0 {
				t.Errorf("%s ist eine Formation, hat aber Ahnen %v", s.ID, syntaxonIDs(ancestors))
			}
			continue
		}
		if len(ancestors) == 0 || ancestors[0].Rank != domain.SyntaxonRankFormation {
			t.Errorf("%s: Ahnenpfad %v beginnt nicht bei einer Formation", s.ID, syntaxonIDs(ancestors))
		}
		if len(ancestors) > maxSyntaxonAncestors {
			t.Errorf("%s: Ahnenpfad hat %d Schritte, erlaubt sind %d",
				s.ID, len(ancestors), maxSyntaxonAncestors)
		}
	}
}

func TestSyntaxonRanksLiefertDieVorhandenenRaengeSortiert(t *testing.T) {
	db := openHierarchyDB(t)

	got, err := db.SyntaxonRanks(t.Context())
	if err != nil {
		t.Fatalf("SyntaxonRanks: %v", err)
	}
	if !slices.Equal(got, []string{"alliance", "class", "formation", "order"}) {
		t.Errorf("Raenge = %v, erwartet [alliance class formation order]", got)
	}
}

func TestSyntaxonRanksEinesLeerenIndexIstLeerUndNichtNil(t *testing.T) {
	db := openTestDB(t)

	got, err := db.SyntaxonRanks(t.Context())
	if err != nil {
		t.Fatalf("SyntaxonRanks: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Raenge = %v, erwartet eine leere, nicht-nil Liste", got)
	}
}
