package application

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func TestIngestSyntaxaFindetElternteilPerNamensabgleich(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		// Name matches FloraVeg's "Testverband", eea code does not.
		eunis: "id,rank,name,parent_id\nAND-01A,alliance,Testverband Morariu 1957,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.ParentsByName != 1 || rep.ParentsDerived != 0 {
		t.Errorf("ParentsByName/Derived = %d/%d, erwartet 1/0",
			rep.ParentsByName, rep.ParentsDerived)
	}
	got := repo.syntaxonByID("AND-01A")
	if got.ParentID != "CA01" || got.ParentProvenance != domain.ParentProvenanceOfficial {
		t.Errorf("AND-01A = %q/%q, erwartet CA01/official", got.ParentID, got.ParentProvenance)
	}
}

func TestIngestSyntaxaLeitetElternteilAusGeschwisterkonsensAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"CA02A,alliance,Nachbarverband eins,A 1,CA02,NAR-01A\n" +
			"CA02B,alliance,Nachbarverband zwei,A 2,CA02,NAR-01B\n" +
			"CA02,order,Nachbarordnung,A 0,CA,NAR-01\n",
		// NAR-01E has no eea-code partner and no name match, but siblings
		// NAR-01A/B unanimously point at CA02.
		eunis: "id,rank,name,parent_id\n" +
			"NAR-01A,alliance,Nachbarverband eins A 1,\n" +
			"NAR-01B,alliance,Nachbarverband zwei A 2,\n" +
			"NAR-01E,alliance,Campanulo-Nardion Rivas-Mart. 1964,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.ParentsDerived != 1 {
		t.Errorf("ParentsDerived = %d, erwartet 1", rep.ParentsDerived)
	}
	got := repo.syntaxonByID("NAR-01E")
	if got.ParentID != "CA02" || got.ParentProvenance != domain.ParentProvenanceDerived {
		t.Errorf("NAR-01E = %q/%q, erwartet CA02/derived", got.ParentID, got.ParentProvenance)
	}
}

// Regression for the real CRU-03 case measured against the full EEA/FloraVeg
// artifacts (Task 9): the file lists two siblings that need sibling
// consensus (NAR-01B, NAR-01C) BEFORE the sibling that resolves by name
// match (NAR-01A) — exactly EEA's own name-sorted order, which does not
// track the EEA id. A single combined pass would leave NAR-01B/C orphaned
// because NAR-01A had not been resolved yet when they were tried; the
// two-pass split (every name match first, then every sibling try) must
// still find the unanimous parent regardless of row order.
func TestIngestSyntaxaLeitetGeschwisterkonsensAbUnabhaengigVonDerDateireihenfolge(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		// ZZZ-01A's own eea code (not "NAR-01A") keeps it out of the
		// eea-code join — NAR-01A must resolve purely via the name match,
		// the path this test exercises.
		hierarchy: minimalHierarchy +
			"CA02,order,Nachbarordnung,A 0,CA,ZZZ-01\n" +
			"CA02A,alliance,Nachbarverband eins,A 1,CA02,ZZZ-01A\n",
		// NAR-01B and NAR-01C precede NAR-01A here — the order the real
		// CRU-03 group's names sorted into in syntaxa.csv, which is not
		// the EEA id order.
		eunis: "id,rank,name,parent_id\n" +
			"NAR-01B,alliance,Voellig anderer Name eins,\n" +
			"NAR-01C,alliance,Voellig anderer Name zwei,\n" +
			"NAR-01A,alliance,Nachbarverband eins A 1,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if rep.ParentsByName != 1 || rep.ParentsDerived != 2 {
		t.Errorf("ParentsByName/Derived = %d/%d, erwartet 1/2", rep.ParentsByName, rep.ParentsDerived)
	}
	if len(rep.Orphans) != 0 {
		t.Errorf("Orphans = %v, erwartet leer", rep.Orphans)
	}
	for _, id := range []string{"NAR-01B", "NAR-01C"} {
		got := repo.syntaxonByID(id)
		if got.ParentID != "CA02" || got.ParentProvenance != domain.ParentProvenanceDerived {
			t.Errorf("%s = %q/%q, erwartet CA02/derived", id, got.ParentID, got.ParentProvenance)
		}
	}
}

func TestIngestSyntaxaLeitetNichtsAusWiderspruechlicherGruppeAb(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"CA02,order,Nachbarordnung,A 0,CA,QUI-01\n" +
			"CA02A,alliance,Erster,A 1,CA02,QUI-01A\n" +
			"RA02,order,Andere Ordnung,A 9,RA,QUI-02\n" +
			"RA02A,alliance,Zweiter,A 2,RA02,QUI-01B\n",
		// QUI-01A -> CA02, QUI-01B -> RA02: group QUI-01 is
		// contradictory, QUI-01F must not inherit anything.
		eunis: "id,rank,name,parent_id\n" +
			"QUI-01A,alliance,Erster A 1,\n" +
			"QUI-01B,alliance,Zweiter A 2,\n" +
			"QUI-01F,alliance,Genisto pilosae-Pinion pinastri,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz verbleibender Waise durch")
	}
	if !strings.Contains(err.Error(), "QUI-01F") {
		t.Errorf("Fehler benennt die Waise nicht: %v", err)
	}
}

func TestIngestSyntaxaScheitertAnVerbleibenderWaise(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		// No eea-code partner, no name match, no sibling.
		eunis: "id,rank,name,parent_id\nEIN-09Z,alliance,Voellig Unbekanntes 1900,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz verbleibender Waise durch")
	}
	if !strings.Contains(err.Error(), "EIN-09Z") {
		t.Errorf("Fehler benennt die Waise nicht: %v", err)
	}
}

func TestIngestSyntaxaLaesstFormationenElternlos(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	rep, err := IngestSyntaxa(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if len(rep.Orphans) != 0 {
		t.Errorf("Orphans = %v, erwartet leer (Formationen zaehlen nicht)", rep.Orphans)
	}
}

func TestIngestSyntaxaMeldetMehrdeutigenNamensabgleich(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations,
		hierarchy: minimalHierarchy +
			"CA02,order,Zweite Ordnung,A 0,CA,ZWE-01\n" +
			"CA02A,alliance,Testverband,Anderer Erhard 1975,CA02,ZWE-01A\n",
		// "Testverband" occurs twice in FloraVeg, same length: no guessing.
		eunis: "id,rank,name,parent_id\nMEH-01A,alliance,Testverband Dritter 1980,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz Waise durch")
	}
	if !strings.Contains(err.Error(), "MEH-01A") {
		t.Errorf("Fehler benennt die Waise nicht: %v", err)
	}
}

// --- Fix-Runde 1: an ambiguous name match no longer blocks the sibling
// try — the two are independent paths (siblings find each other over the
// eea code, not the name), and a real unanimous answer must win over an
// avoidable orphan. Both tests call assignRemainingParents directly
// because IngestSyntaxa discards its SyntaxaReport on a failed run, and
// the second case needs to inspect AmbiguousMatches and Orphans on exactly
// that outcome. ---

func TestAssignRemainingParentsLoestMehrdeutigenNamensabgleichUeberGeschwisterkonsensAuf(t *testing.T) {
	repo := newFakeRepo()
	// SIB-01E must already exist for SetSyntaxonParent to find it — mirrors
	// what writeEunisOnly would have written in step 3.
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{ID: "SIB-01E", Rank: domain.SyntaxonRankAlliance})
	rows := []hierarchyRow{
		// Two FloraVeg alliances share the name "Testverband": the name
		// match is ambiguous. Their eea codes are SIB-01E's siblings
		// (SIB-01) and unanimously point at CA02.
		{code: "CA02A", rank: domain.SyntaxonRankAlliance, name: "Testverband", parentCode: "CA02", eeaCode: "SIB-01A"},
		{code: "CA02B", rank: domain.SyntaxonRankAlliance, name: "Testverband", parentCode: "CA02", eeaCode: "SIB-01B"},
	}
	rep := &SyntaxaReport{}
	eunisOnly := []eunisOnlyRow{{id: "SIB-01E", name: "Testverband Dritter 1980"}}

	if _, err := assignRemainingParents(repo, eunisOnly, rows, rep); err != nil {
		t.Fatalf("assignRemainingParents: %v", err)
	}
	if rep.ParentsDerived != 1 {
		t.Errorf("ParentsDerived = %d, erwartet 1", rep.ParentsDerived)
	}
	if len(rep.AmbiguousMatches) != 1 || rep.AmbiguousMatches[0] != "SIB-01E" {
		t.Errorf("AmbiguousMatches = %v, erwartet [SIB-01E]", rep.AmbiguousMatches)
	}
	if len(rep.Orphans) != 0 {
		t.Errorf("Orphans = %v, erwartet leer — der Geschwisterkonsens hat entschieden", rep.Orphans)
	}
	got := repo.syntaxonByID("SIB-01E")
	if got.ParentID != "CA02" || got.ParentProvenance != domain.ParentProvenanceDerived {
		t.Errorf("SIB-01E = %q/%q, erwartet CA02/derived", got.ParentID, got.ParentProvenance)
	}
}

func TestAssignRemainingParentsBleibtWaiseBeiMehrdeutigemNamensabgleichOhneKonsens(t *testing.T) {
	repo := newFakeRepo()
	rows := []hierarchyRow{
		// Same ambiguous name collision, but this time without eea codes:
		// no siblings exist for MEH-02Z's group at all.
		{code: "CA02A", rank: domain.SyntaxonRankAlliance, name: "Testverband", parentCode: "CA02"},
		{code: "CA03A", rank: domain.SyntaxonRankAlliance, name: "Testverband", parentCode: "CA03"},
	}
	rep := &SyntaxaReport{}
	eunisOnly := []eunisOnlyRow{{id: "MEH-02Z", name: "Testverband Dritter 1980"}}

	if _, err := assignRemainingParents(repo, eunisOnly, rows, rep); err != nil {
		t.Fatalf("assignRemainingParents: %v", err)
	}
	if len(rep.AmbiguousMatches) != 1 || rep.AmbiguousMatches[0] != "MEH-02Z" {
		t.Errorf("AmbiguousMatches = %v, erwartet [MEH-02Z]", rep.AmbiguousMatches)
	}
	if len(rep.Orphans) != 1 || rep.Orphans[0] != "MEH-02Z" {
		t.Errorf("Orphans = %v, erwartet [MEH-02Z] — kein Konsens, also bleibt es eine Waise", rep.Orphans)
	}
}

// The two-pass split (Task 9's order-independence fix) gives
// setResolvedParent two distinct call sites in assignRemainingParents — the
// name-match pass and the sibling-consensus pass. TestIngestSyntaxaMeldetFehlerBeimSetzenDesElternteils
// below only reaches the first (AND-01A resolves by name); this pins the
// second by giving the row no name match at all, only a unanimous sibling.
func TestAssignRemainingParentsMeldetFehlerBeimSetzenDesElternteilsUeberGeschwisterkonsens(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "SetSyntaxonParent"
	repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{ID: "NAR-01E", Rank: domain.SyntaxonRankAlliance})
	rows := []hierarchyRow{
		{code: "CA02A", rank: domain.SyntaxonRankAlliance, name: "Nachbarverband eins", parentCode: "CA02", eeaCode: "NAR-01A"},
	}
	rep := &SyntaxaReport{}
	eunisOnly := []eunisOnlyRow{{id: "NAR-01E", name: "Voellig anderer Name ohne Treffer"}}

	_, err := assignRemainingParents(repo, eunisOnly, rows, rep)
	if err == nil {
		t.Fatal("assignRemainingParents lief trotz fehlschlagendem SetSyntaxonParent durch")
	}
	if !strings.Contains(err.Error(), "setting parent of NAR-01E") {
		t.Errorf("Fehler = %v, erwartet SetSyntaxonParent-Kontext", err)
	}
}

// A collision in the eea_code column no longer reaches this pass at all:
// IngestSyntaxa fails on it before the transaction opens (checkEEACodeCollisions,
// pinned by TestIngestSyntaxaScheitertAnKollidierendemEEACode), so the parent
// map has no ambiguous key left to exclude.

// --- Coverage for the paths the six brief tests do not reach: error
// wrapping from SetSyntaxonParent. ---

func TestIngestSyntaxaMeldetFehlerBeimSetzenDesElternteils(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "SetSyntaxonParent"
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\nAND-01A,alliance,Testverband Morariu 1957,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz fehlschlagendem SetSyntaxonParent durch")
	}
	if !strings.Contains(err.Error(), "setting parent of AND-01A") {
		t.Errorf("Fehler = %v, erwartet SetSyntaxonParent-Kontext", err)
	}
}

func TestIngestSyntaxaMeldetFehlerBeimLeerenDerSyntaxa(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "ClearSyntaxa"
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz fehlschlagendem ClearSyntaxa durch")
	}
	if !strings.Contains(err.Error(), "clearing syntaxa before ingest") {
		t.Errorf("Fehler = %v, erwartet ClearSyntaxa-Kontext", err)
	}
}

// siblingConsensus's own group check (id has no EEA group at all) is a
// different branch than eeaGroup's unit tests below: it only fires when
// the row survives the name-match step first and then asks for consensus.
func TestIngestSyntaxaOrphantOhneEeaGruppe(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		// "XX" has no valid EEA group (no trailing letter+digit): no name
		// match, no sibling consensus possible at all.
		eunis: "id,rank,name,parent_id\nXX,alliance,Voellig Unbekanntes 1900,\n",
		links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz verbleibender Waise durch")
	}
	if !strings.Contains(err.Error(), "XX") {
		t.Errorf("Fehler benennt die Waise nicht: %v", err)
	}
}

// --- Direct unit tests for longestPrefixMatch and eeaGroup: exercising
// branches (empty candidate name, no match at all, the word-boundary
// rejection, malformed EEA ids) that the ingest-level scenarios above do
// not happen to reach. ---

func TestLongestPrefixMatchVerlangtWortgrenze(t *testing.T) {
	candidates := []hierarchyRow{{name: "Salicion alba", parentCode: "X"}}
	match, ambiguous := longestPrefixMatch("Salicion albae Soo 1930", candidates)
	if match != nil || ambiguous {
		t.Errorf("match/ambiguous = %v/%v, erwartet nil/false (kein Wortgrenzen-Treffer)", match, ambiguous)
	}
}

func TestLongestPrefixMatchLiefertKeinenTreffer(t *testing.T) {
	candidates := []hierarchyRow{{name: "Andere Sache", parentCode: "X"}}
	match, ambiguous := longestPrefixMatch("Voellig Unbekanntes", candidates)
	if match != nil || ambiguous {
		t.Errorf("match/ambiguous = %v/%v, erwartet nil/false", match, ambiguous)
	}
}

func TestLongestPrefixMatchUeberspringtLeerenNamen(t *testing.T) {
	candidates := []hierarchyRow{{name: "", parentCode: "X"}}
	match, ambiguous := longestPrefixMatch("Irgendwas", candidates)
	if match != nil || ambiguous {
		t.Errorf("match/ambiguous = %v/%v, erwartet nil/false", match, ambiguous)
	}
}

// eunisName equal to c.name (no author suffix at all) is the boundary of the
// word-boundary check: len(eunisName) > len(c.name) is false, so the index
// into eunisName is never taken. Pins the `>` in that condition against a
// `>=` mutant, which would try to index eunisName one past its end.
func TestLongestPrefixMatchTrifftBeiExaktGleichLangenNamen(t *testing.T) {
	candidates := []hierarchyRow{{name: "Testverband", parentCode: "CA01"}}
	match, ambiguous := longestPrefixMatch("Testverband", candidates)
	if ambiguous || match == nil || match.parentCode != "CA01" {
		t.Errorf("match/ambiguous = %v/%v, erwartet CA01/false", match, ambiguous)
	}
}

// A single candidate whose name is exactly one rune long pins bestLen's
// initial value (-1): mutated to 0 or 1, the very first candidate would tie
// against the starting value instead of beating it, wrongly reporting
// ambiguity for the only candidate that exists.
func TestLongestPrefixMatchTrifftBeiEinbuchstabigemNamenOhneAmbiguitaet(t *testing.T) {
	candidates := []hierarchyRow{{name: "X", parentCode: "CA01"}}
	match, ambiguous := longestPrefixMatch("X", candidates)
	if ambiguous || match == nil || match.parentCode != "CA01" {
		t.Errorf("match/ambiguous = %v/%v, erwartet CA01/false", match, ambiguous)
	}
}

func TestEeaGroupErkenntKeineGruppeBeiZuKurzerID(t *testing.T) {
	if _, ok := eeaGroup("A"); ok {
		t.Error("eeaGroup(\"A\") = ok, erwartet keine Gruppe")
	}
}

// The shortest id the letter/digit checks could still accept is length 2 —
// pins the `<` in `len(id) < 2` against a `<=` mutant, which would reject
// this id on length alone despite it otherwise being valid.
func TestEeaGroupErkenntGruppeBeiMinimalerGueltigerLaenge(t *testing.T) {
	got, ok := eeaGroup("9A")
	if !ok || got != "9" {
		t.Errorf("eeaGroup(\"9A\") = %q/%v, erwartet 9/true", got, ok)
	}
}

// The brief's own cautionary example: ASP-03 must not become group "ASP-0"
// — the last character is a digit, not the alliance letter, so an order
// code must never get a group at all.
func TestEeaGroupErkenntKeineGruppeBeiOrdnungscode(t *testing.T) {
	if group, ok := eeaGroup("ASP-03"); ok {
		t.Errorf("eeaGroup(\"ASP-03\") = %q, erwartet keine Gruppe", group)
	}
}

func TestEeaGroupErkenntKeineGruppeOhneZifferDavor(t *testing.T) {
	if group, ok := eeaGroup("AND-XXA"); ok {
		t.Errorf("eeaGroup(\"AND-XXA\") = %q, erwartet keine Gruppe", group)
	}
}

func TestEeaGroupLiefertGruppeFuerGueltigeID(t *testing.T) {
	got, ok := eeaGroup("NAR-01E")
	if !ok || got != "NAR-01" {
		t.Errorf("eeaGroup(\"NAR-01E\") = %q/%v, erwartet NAR-01/true", got, ok)
	}
}

// eeaGroup's letter/digit range checks pin all four boundary values: 'A' and
// 'Z' are the smallest/largest valid alliance letters, '0' and '9' the
// smallest/largest valid digit before it. A `<`/`>` mutated to `<=`/`>=`
// changes behavior only exactly at these values.
func TestEeaGroupErkenntGrenzwerteAlsGueltig(t *testing.T) {
	for _, id := range []string{"XX-00A", "XX-99Z"} {
		if _, ok := eeaGroup(id); !ok {
			t.Errorf("eeaGroup(%q) = keine Gruppe, erwartet eine (Grenzwert gueltig)", id)
		}
	}
}
