package application

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// seedNavRepo builds the world the navigation tests ask about: a phanerogam
// chain C -> CA -> CA01 -> CA01A/CA01B, a cryptogam chain R -> RA -> RA01 ->
// RA01A, and two habitat-type edges on CA01A.
func seedNavRepo(t *testing.T) *fakeRepo {
	t.Helper()
	repo := newFakeRepo()
	repo.syntaxa = []domain.Syntaxon{
		{ID: "C", Rank: domain.SyntaxonRankFormation, Name: "Vegetation of the nemoral forest zone",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial,
			LifeFormGroup: domain.LifeFormPhanerogam},
		{ID: "CA", Rank: domain.SyntaxonRankClass, Name: "Testklasse", Author: "Moor 1950",
			ParentID: "C", EEACode: "TST", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01", Rank: domain.SyntaxonRankOrder, Name: "Testordnung", Author: "Moor 1960",
			ParentID: "CA", EEACode: "TST-01", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01A", Rank: domain.SyntaxonRankAlliance, Name: "Erster Verband", Author: "Moor 1970",
			ParentID: "CA01", EEACode: "TST-01A", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "CA01B", Rank: domain.SyntaxonRankAlliance, Name: "Zweiter Verband", Author: "Moor 1975",
			ParentID: "CA01", Source: domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "R", Rank: domain.SyntaxonRankFormation, Name: "Epigaeic bryophyte and lichen vegetation",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial,
			LifeFormGroup: domain.LifeFormBryophyteLichen},
		{ID: "RA", Rank: domain.SyntaxonRankClass, Name: "Moosklasse", ParentID: "R",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA01", Rank: domain.SyntaxonRankOrder, Name: "Moosordnung", ParentID: "RA",
			Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial},
		{ID: "RA01A", Rank: domain.SyntaxonRankAlliance, Name: "Moosverband", ParentID: "RA01",
			Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceDerived},
	}
	for _, code := range []string{"T11", "T12"} {
		if err := repo.LinkSyntaxon(domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}, "CA01A"); err != nil {
			t.Fatalf("LinkSyntaxon(%s): %v", code, err)
		}
	}
	return repo
}

func refIDs(in []input.SyntaxonRef) []string {
	out := make([]string, 0, len(in))
	for _, r := range in {
		out = append(out, r.ID)
	}
	return out
}

func TestSyntaxon_FormationHatLeerenAhnenpfadUndIhreKlassenAlsKinder(t *testing.T) {
	q := NewQueryService(seedNavRepo(t))

	got, err := q.Syntaxon(t.Context(), "C", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Ancestors == nil || len(got.Ancestors) != 0 {
		t.Errorf("Ancestors = %v, erwartet leer und nicht nil", got.Ancestors)
	}
	if !slices.Equal(refIDs(got.Children), []string{"CA"}) {
		t.Errorf("Children = %v, erwartet [CA]", refIDs(got.Children))
	}
	if got.LifeFormGroup != domain.LifeFormPhanerogam {
		t.Errorf("LifeFormGroup = %q, erwartet phanerogam (auf der Zeile gespeichert)", got.LifeFormGroup)
	}
	if got.DirectHabitatTypeCount != 0 {
		t.Errorf("DirectHabitatTypeCount = %d, erwartet 0", got.DirectHabitatTypeCount)
	}
}

func TestSyntaxon_KlasseUndOrdnungTragenDenPfadAeussersteZuerst(t *testing.T) {
	q := NewQueryService(seedNavRepo(t))

	class, err := q.Syntaxon(t.Context(), "CA", "en")
	if err != nil {
		t.Fatalf("Syntaxon(CA): %v", err)
	}
	if !slices.Equal(refIDs(class.Ancestors), []string{"C"}) {
		t.Errorf("Ancestors(CA) = %v, erwartet [C]", refIDs(class.Ancestors))
	}
	order, err := q.Syntaxon(t.Context(), "CA01", "en")
	if err != nil {
		t.Fatalf("Syntaxon(CA01): %v", err)
	}
	if !slices.Equal(refIDs(order.Ancestors), []string{"C", "CA"}) {
		t.Errorf("Ancestors(CA01) = %v, erwartet [C CA]", refIDs(order.Ancestors))
	}
	if !slices.Equal(refIDs(order.Children), []string{"CA01A", "CA01B"}) {
		t.Errorf("Children(CA01) = %v, erwartet [CA01A CA01B]", refIDs(order.Children))
	}
}

func TestSyntaxon_VerbandHatDreiAhnenKeineKinderUndSeineKantenzahl(t *testing.T) {
	q := NewQueryService(seedNavRepo(t))

	got, err := q.Syntaxon(t.Context(), "CA01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if !slices.Equal(refIDs(got.Ancestors), []string{"C", "CA", "CA01"}) {
		t.Errorf("Ancestors = %v, erwartet [C CA CA01]", refIDs(got.Ancestors))
	}
	if got.Children == nil || len(got.Children) != 0 {
		t.Errorf("Children = %v, erwartet leer und nicht nil", got.Children)
	}
	if got.DirectHabitatTypeCount != 2 {
		t.Errorf("DirectHabitatTypeCount = %d, erwartet 2", got.DirectHabitatTypeCount)
	}
	if got.EEACode != "TST-01A" || got.Source != domain.SyntaxonSourceEVC ||
		got.ParentProvenance != domain.ParentProvenanceOfficial || got.Author != "Moor 1970" {
		t.Errorf("SyntaxonRef = %+v, erwartet die Herkunftsfelder aus Teilprojekt A", got.SyntaxonRef)
	}
}

func TestSyntaxon_LeitetDieLebensformGruppeAusDerFormationAb(t *testing.T) {
	q := NewQueryService(seedNavRepo(t))

	got, err := q.Syntaxon(t.Context(), "RA01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	// The value sits on formation row R alone. Derived, not read — and
	// verifiable, because ancestors travels in the same answer.
	if got.LifeFormGroup != domain.LifeFormBryophyteLichen {
		t.Errorf("LifeFormGroup = %q, erwartet bryophyte_lichen aus Formation R", got.LifeFormGroup)
	}
	if len(got.Ancestors) == 0 || got.Ancestors[0].ID != "R" {
		t.Errorf("Ancestors = %v, erwartet R an erster Stelle", refIDs(got.Ancestors))
	}
}

func TestSyntaxon_FormationOhneLebensformGruppeBleibtLeer(t *testing.T) {
	repo := seedNavRepo(t)
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "X", Rank: domain.SyntaxonRankFormation, Name: "Unclassified formation",
		Source: domain.SyntaxonSourceEVC, ParentProvenance: domain.ParentProvenanceOfficial})
	q := NewQueryService(repo)

	got, err := q.Syntaxon(t.Context(), "X", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	// A formation is the only rank with an empty ancestor path, and this one
	// carries no life-form group of its own: nothing to derive it from, so it
	// must stay empty rather than invent a value.
	if got.LifeFormGroup != "" {
		t.Errorf("LifeFormGroup = %q, erwartet leer (keine Formation zum Ableiten)", got.LifeFormGroup)
	}
}

func TestSyntaxon_UnbekannteIDIstNotFound(t *testing.T) {
	q := NewQueryService(seedNavRepo(t))

	_, err := q.Syntaxon(t.Context(), "GIBTSNICHT", "en")
	if !errors.Is(err, input.ErrNotFound) {
		t.Errorf("Fehler = %v, erwartet input.ErrNotFound", err)
	}
}

// TestSyntaxon_FallbackFehlerBleibtInternalErrorStattNotFound covers the
// case Copilot flagged in PR #51: the primary lookup genuinely misses (a
// plain output.ErrNotFound, e.g. an id that no longer exists), and the
// eea_code fallback THEN fails for an unrelated reason — a broken index
// read, not "also not found". That must not collapse into NOT_FOUND: the
// route would answer 404 while the index itself is unreadable, exactly the
// dangling-parent_id confusion this file's comment above rejects a few
// lines up.
func TestSyntaxon_FallbackFehlerBleibtInternalErrorStattNotFound(t *testing.T) {
	repo := seedNavRepo(t)
	boom := errors.New("index unreadable")
	repo.syntaxonByEEACodeErr = boom
	q := NewQueryService(repo)

	_, err := q.Syntaxon(t.Context(), "GIBTSNICHT", "en")
	if err == nil {
		t.Fatal("Syntaxon: erwartet einen Fehler")
	}
	if errors.Is(err, input.ErrNotFound) {
		t.Errorf("Fehler = %v, erwartet KEIN input.ErrNotFound (der Fallback-Fehler ist kein Nicht-gefunden)", err)
	}
}

// TestSyntaxon_MeldetEchtenRepositoryFehlerOhneEEACodeFallback covers the
// third branch of syntaxonByIDOrEEACode: a plain repository failure (not
// output.ErrNotFound) must surface as-is and must NOT trigger the eea_code
// fallback lookup — that lookup is only for a genuinely missing id.
func TestSyntaxon_MeldetEchtenRepositoryFehlerOhneEEACodeFallback(t *testing.T) {
	repo := seedNavRepo(t)
	boom := errors.New("boom")
	repo.syntaxonErr = boom
	q := NewQueryService(repo)

	_, err := q.Syntaxon(t.Context(), "CA01A", "en")
	if err == nil || errors.Is(err, input.ErrNotFound) {
		t.Errorf("Fehler = %v, erwartet einen durchgereichten Repository-Fehler, kein input.ErrNotFound", err)
	}
}

// TestSyntaxon_FindetUeberDenEEACodeWennDieIDNichtMehrExistiert covers the
// fallback this task added: a client still holding the old EEA-EUNIS id
// (e.g. TST-01A) must find the same unit under its current EVC id (CA01A).
func TestSyntaxon_FindetUeberDenEEACodeWennDieIDNichtMehrExistiert(t *testing.T) {
	q := NewQueryService(seedNavRepo(t))

	got, err := q.Syntaxon(t.Context(), "TST-01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.ID != "CA01A" {
		t.Errorf("ID = %q, erwartet CA01A (aufgeloest ueber eea_code TST-01A)", got.ID)
	}
	if !slices.Equal(refIDs(got.Ancestors), []string{"C", "CA", "CA01"}) {
		t.Errorf("Ancestors = %v, erwartet den Pfad ab der aufgeloesten ID", refIDs(got.Ancestors))
	}
}

// TestSyntaxon_IDGewinntVorDemEEACodeFallback covers the precedence rule: a
// value that exists as a syntaxon's own id must resolve to THAT syntaxon,
// even when it also happens to be a different syntaxon's eea_code. The
// fallback lookup must not even run — the fake repo's SyntaxonByEEACode
// would return the wrong unit if it did.
func TestSyntaxon_IDGewinntVorDemEEACodeFallback(t *testing.T) {
	repo := seedNavRepo(t)
	repo.syntaxa = append(repo.syntaxa,
		domain.Syntaxon{ID: "TST-01A", Rank: domain.SyntaxonRankAlliance, Name: "Eigene ID gleich fremdem eea_code",
			ParentID: "CA01", Source: domain.SyntaxonSourceEUNIS, ParentProvenance: domain.ParentProvenanceOfficial})
	q := NewQueryService(repo)

	got, err := q.Syntaxon(t.Context(), "TST-01A", "en")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.Name != "Eigene ID gleich fremdem eea_code" {
		t.Errorf("Name = %q, erwartet den Treffer ueber die ID, nicht ueber den Fallback", got.Name)
	}
}

func TestSyntaxon_BaumelndeElternreferenzIstEinInkonsistenterIndex(t *testing.T) {
	repo := seedNavRepo(t)
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
		ID: "WAISE", Rank: domain.SyntaxonRankAlliance, Name: "Zeigt ins Leere",
		ParentID: "FEHLT", Source: domain.SyntaxonSourceEUNIS,
		ParentProvenance: domain.ParentProvenanceDerived})
	q := NewQueryService(repo)

	_, err := q.Syntaxon(t.Context(), "WAISE", "en")
	if err == nil {
		t.Fatal("Syntaxon hat eine baumelnde parent_id ueberbrueckt")
	}
	if !strings.Contains(err.Error(), "index is inconsistent") {
		t.Errorf("Fehler = %v, erwartet die Inkonsistenz-Meldung", err)
	}
	// A 404 would be wrong: WAISE exists, the index is broken.
	if errors.Is(err, input.ErrNotFound) {
		t.Error("der Fehler wird als NOT_FOUND klassifiziert, erwartet INTERNAL_ERROR")
	}
}

func TestSyntaxon_MeldetFehlerDerKinderAbfrage(t *testing.T) {
	repo := seedNavRepo(t)
	repo.syntaxonChildrenErr = errors.New("boom")
	q := NewQueryService(repo)

	if _, err := q.Syntaxon(t.Context(), "CA01", "en"); err == nil {
		t.Fatal("Syntaxon verschluckt den Fehler der Kinder-Abfrage")
	}
}

func TestSyntaxon_MeldetFehlerDerKantenzaehlung(t *testing.T) {
	repo := seedNavRepo(t)
	repo.habitatTypeCountErr = errors.New("boom")
	q := NewQueryService(repo)

	if _, err := q.Syntaxon(t.Context(), "CA01A", "en"); err == nil {
		t.Fatal("Syntaxon verschluckt den Fehler der Kantenzaehlung")
	}
}

func TestSyntaxaByRank_ListetEinenRang(t *testing.T) {
	q := NewQueryService(seedNavRepo(t))

	got, err := q.SyntaxaByRank(t.Context(), domain.SyntaxonRankClass, "", "", input.SyntaxonAreaFilter{})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(refIDs(got), []string{"CA", "RA"}) {
		t.Errorf("Klassen = %v, erwartet [CA RA]", refIDs(got))
	}
}

func TestSyntaxaByRank_KombiniertRangUndGruppeAlsUnd(t *testing.T) {
	q := NewQueryService(seedNavRepo(t))

	got, err := q.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, domain.LifeFormBryophyteLichen, "", input.SyntaxonAreaFilter{})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if !slices.Equal(refIDs(got), []string{"RA01A"}) {
		t.Errorf("Moosverbaende = %v, erwartet [RA01A]", refIDs(got))
	}
}

func TestSyntaxaByRank_UnbekannterRangIstInvalidQueryMitDenErlaubtenWerten(t *testing.T) {
	q := NewQueryService(seedNavRepo(t))

	_, err := q.SyntaxaByRank(t.Context(), "association", "", "", input.SyntaxonAreaFilter{})
	if !errors.Is(err, input.ErrInvalidQuery) {
		t.Fatalf("Fehler = %v, erwartet input.ErrInvalidQuery", err)
	}
	// An empty list would be the wrong answer: it looks like "there are none"
	// while it means "you mistyped".
	for _, want := range []string{"alliance", "class", "formation", "order"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Nachricht %q nennt den erlaubten Wert %q nicht", err.Error(), want)
		}
	}
}

func TestSyntaxaByRank_MeldetFehlerDerRanglisteUndDerAbfrage(t *testing.T) {
	ranksBroken := seedNavRepo(t)
	ranksBroken.syntaxonRanksErr = errors.New("boom")
	if _, err := NewQueryService(ranksBroken).
		SyntaxaByRank(t.Context(), domain.SyntaxonRankClass, "", "", input.SyntaxonAreaFilter{}); err == nil {
		t.Error("SyntaxaByRank verschluckt den Fehler der Rangliste")
	}

	queryBroken := seedNavRepo(t)
	queryBroken.syntaxaByRankErr = errors.New("boom")
	if _, err := NewQueryService(queryBroken).
		SyntaxaByRank(t.Context(), domain.SyntaxonRankClass, "", "", input.SyntaxonAreaFilter{}); err == nil {
		t.Error("SyntaxaByRank verschluckt den Fehler der Abfrage")
	}
}
