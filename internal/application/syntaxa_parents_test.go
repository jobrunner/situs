package application

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func TestIngestSyntaxaFindetElternteilPerNamensabgleich(t *testing.T) {
	repo := newFakeRepo()
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		// Name matches FloraVeg's "Testverband", alt code does not.
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
		// NAR-01E has no alt-code partner and no name match, but siblings
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
		// No alt-code partner, no name match, no sibling.
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

// --- Coverage for the paths the six brief tests do not reach: error
// wrapping from SetSyntaxonParent, the pre-Begin altcode read, and the
// idempotent RelinkSyntaxon nachlauf. ---

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

func TestIngestSyntaxaMeldetFehlerBeimLesenDerAltcodes(t *testing.T) {
	repo := newFakeRepo()
	repo.altCodesErr = fmt.Errorf("boom")
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz Fehler beim Altcode-Lesen durch")
	}
	if !strings.Contains(err.Error(), "reading existing syntaxon alt codes") {
		t.Errorf("Fehler = %v, erwartet Altcode-Kontext", err)
	}
}

func TestIngestSyntaxaVerknuepftAltenAltcodeMitDemNeuenPrimaercode(t *testing.T) {
	repo := newFakeRepo()
	// OLD-01A carries an edge from a run before FloraVeg claimed TST-01A —
	// the repeat-ingest case the nachlauf repairs. NOPE-01A has no
	// counterpart in this run's hierarchy at all: present in the index's
	// existing state, but not a key of this run's byAlt map, so it must be
	// left alone (the !ok branch of relinkStaleAltCodes).
	repo.syntaxa = append(repo.syntaxa,
		domain.Syntaxon{ID: "OLD-01A", Rank: domain.SyntaxonRankAlliance, AltCode: "TST-01A"},
		domain.Syntaxon{ID: "ZZZ", Rank: domain.SyntaxonRankAlliance, AltCode: "NOPE-01A"},
	)
	repo.syntaxaLinks = append(repo.syntaxaLinks, struct {
		key        domain.HabitatTypeKey
		syntaxonID string
	}{key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: "T11"}, syntaxonID: "OLD-01A"})
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	if _, err := IngestSyntaxa(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestSyntaxa: %v", err)
	}
	if len(repo.syntaxaLinks) != 1 || repo.syntaxaLinks[0].syntaxonID != "CA01A" {
		t.Errorf("syntaxaLinks = %+v, erwartet genau eine Kante auf CA01A nach dem Relink-Nachlauf", repo.syntaxaLinks)
	}
}

func TestIngestSyntaxaMeldetFehlerBeimRelinkNachlauf(t *testing.T) {
	repo := newFakeRepo()
	repo.syntaxa = append(repo.syntaxa,
		domain.Syntaxon{ID: "OLD-01A", Rank: domain.SyntaxonRankAlliance, AltCode: "TST-01A"},
	)
	repo.failOn = "RelinkSyntaxon"
	dir := writeSyntaxaDir(t, syntaxaFiles{
		formations: minimalFormations, hierarchy: minimalHierarchy,
		eunis: "id,rank,name,parent_id\n", links: "typology_id,code,syntaxon_id\n",
	})
	_, err := IngestSyntaxa(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestSyntaxa lief trotz fehlschlagendem RelinkSyntaxon durch")
	}
	if !strings.Contains(err.Error(), "relinking OLD-01A to CA01A") {
		t.Errorf("Fehler = %v, erwartet Relink-Kontext", err)
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

func TestEeaGroupErkenntKeineGruppeBeiZuKurzerID(t *testing.T) {
	if _, ok := eeaGroup("A"); ok {
		t.Error("eeaGroup(\"A\") = ok, erwartet keine Gruppe")
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
