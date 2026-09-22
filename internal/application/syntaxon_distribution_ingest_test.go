package application

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// errBoom is an injected failure sentinel, distinct from any real error the
// code under test could produce, so a test asserting propagation cannot pass
// by coincidence.
var errBoom = errors.New("boom")

const (
	minimalCoverage = "syntaxon_id,area_scheme\n" +
		"CA01A,evc_territory\n" +
		"CA01B,evc_territory\n"
	minimalDistribution = "syntaxon_id,area_scheme,area_code,occurrence\n" +
		"CA01A,evc_territory,austria-alps,verified\n" +
		"CA01A,evc_territory,czech-republic,uncertain\n"
)

// seedSyntaxa puts the ids the distribution rows refer to into the fake repo,
// the way IngestSyntaxa would have.
func seedSyntaxa(repo *fakeRepo, ids ...string) {
	for _, id := range ids {
		repo.syntaxa = append(repo.syntaxa, domain.Syntaxon{
			ID: id, Rank: domain.SyntaxonRankAlliance, Name: id,
			Source: domain.SyntaxonSourceEVC,
		})
	}
}

func writeDistFiles(t *testing.T, dist, coverage string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	distPath := filepath.Join(dir, "syntaxon_distribution.csv")
	covPath := filepath.Join(dir, "syntaxon_distribution_coverage.csv")
	for path, content := range map[string]string{distPath: dist, covPath: coverage} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("Schreiben von %s: %v", path, err)
		}
	}
	return distPath, covPath
}

// occurrence, covered and occurrenceCount read fakeRepo's own recorded state
// (syntaxonDistribution/syntaxonCoverage, added by Task 5) rather than a
// second map — the fake's storage is a slice, so these walk it.
func (r *fakeRepo) occurrence(id, scheme, code string) string {
	for _, o := range r.syntaxonDistribution {
		if o.SyntaxonID == id && o.Scheme == scheme && o.Code == code {
			return o.Occurrence
		}
	}
	return ""
}

func (r *fakeRepo) covered(id, scheme string) bool {
	for _, c := range r.syntaxonCoverage {
		if c.SyntaxonID == id && c.Scheme == scheme {
			return true
		}
	}
	return false
}

func (r *fakeRepo) occurrenceCount(id string) int {
	n := 0
	for _, o := range r.syntaxonDistribution {
		if o.SyntaxonID == id {
			n++
		}
	}
	return n
}

func TestIngestSyntaxonDistributionFuelltBeideTabellen(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 2 || rep.Verified != 1 || rep.Uncertain != 1 || rep.Covered != 2 {
		t.Errorf("Report = %+v, erwartet Written 2 / Verified 1 / Uncertain 1 / Covered 2", rep)
	}
	if got := repo.occurrence("CA01A", "evc_territory", "austria-alps"); got != domain.OccurrenceVerified {
		t.Errorf("austria-alps = %q, erwartet verified", got)
	}
	if got := repo.occurrence("CA01A", "evc_territory", "czech-republic"); got != domain.OccurrenceUncertain {
		t.Errorf("czech-republic = %q, erwartet uncertain", got)
	}
}

// A symmetric mix (one verified, one uncertain, as minimalDistribution has)
// cannot catch a mutant that swaps which counter a row increments: with
// exactly one of each, Verified/Uncertain end up 1/1 either way. This test
// uses two verified and one uncertain row instead, so a swapped comparison
// at `occurrence == domain.OccurrenceUncertain` changes the aggregate counts,
// not just which row contributed to which.
func TestIngestSyntaxonDistributionZaehltVerifiedUndUncertainGetrennt(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc_territory,austria-alps,verified\n"+
			"CA01A,evc_territory,albania,verified\n"+
			"CA01A,evc_territory,czech-republic,uncertain\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 3 || rep.Verified != 2 || rep.Uncertain != 1 {
		t.Errorf("Report = %+v, erwartet Written 3 / Verified 2 / Uncertain 1", rep)
	}
}

func TestIngestSyntaxonDistributionBehaeltCoverageOhneVorkommen(t *testing.T) {
	// CA01B has a coverage row and no occurrence row. That is a STATEMENT
	// ("checked, occurs in no territory") and the row must survive — it is the
	// only thing that keeps absence apart from unknown.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Covered != 2 {
		t.Fatalf("Covered = %d, erwartet 2", rep.Covered)
	}
	if !repo.covered("CA01B", "evc_territory") {
		t.Error("CA01B hat keine Coverage-Zeile, obwohl die Quelle eine Aussage macht")
	}
	if n := repo.occurrenceCount("CA01B"); n != 0 {
		t.Errorf("CA01B traegt %d Vorkommen, erwartet 0", n)
	}
}

func TestIngestSyntaxonDistributionVerwirftUnbekanntenCodeUndMeldetIhn(t *testing.T) {
	// The measured case: CI01E is in the distribution file (EVC fassung 3) but
	// not in the pinned FloraVeg file. Real fassung drift between two sources,
	// not a defect of this pipeline — so it is named, not rounded away.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		minimalDistribution+"CI01E,evc_territory,spain-atlantic,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\nCI01E,evc_territory\n")

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if len(rep.UnknownSyntaxa) != 1 || rep.UnknownSyntaxa[0] != "CI01E" {
		t.Errorf("UnknownSyntaxa = %v, erwartet [CI01E]", rep.UnknownSyntaxa)
	}
	if rep.Written != 2 {
		t.Errorf("Written = %d, erwartet 2 — die CI01E-Zeile darf nicht geschrieben werden", rep.Written)
	}
	if rep.Covered != 1 {
		t.Errorf("Covered = %d, erwartet 1 — auch die Coverage-Zeile faellt weg", rep.Covered)
	}
	if repo.covered("CI01E", "evc_territory") {
		t.Error("CI01E bekam eine Coverage-Zeile, obwohl der Index das Syntaxon nicht kennt")
	}
}

func TestIngestSyntaxonDistributionMeldetJedenUnbekanntenCodeGenauEinmal(t *testing.T) {
	// One unknown alliance has up to 136 rows. Reporting it 136 times would
	// turn one fassung drift into a wall of noise.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CI01E,evc_territory,spain-atlantic,verified\n"+
			"CI01E,evc_territory,portugal-mediterranean,verified\n"+
			"CI01E,evc_territory,spain-mediterranean,verified\n",
		"syntaxon_id,area_scheme\nCI01E,evc_territory\n")

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if len(rep.UnknownSyntaxa) != 1 {
		t.Errorf("UnknownSyntaxa = %v, erwartet genau einen Eintrag", rep.UnknownSyntaxa)
	}
}

// The two warning branches in IngestSyntaxonDistribution (unknown syntaxa,
// skipped rows) have no side effect a report field can catch — only the log
// line does. Both directions matter: a boundary or negation mutant on either
// `> 0` would fire the warning on the clean run below, or stay silent on the
// dirty one.
func TestIngestSyntaxonDistributionWarntNurBeiTatsaechlichenBefunden(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")

	var clean bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&clean, nil)))
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)
	if _, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov); err != nil {
		slog.SetDefault(prev)
		t.Fatalf("IngestSyntaxonDistribution (clean): %v", err)
	}
	slog.SetDefault(prev)
	if got := clean.String(); strings.Contains(got, "does not carry") || strings.Contains(got, "skipped malformed rows") {
		t.Errorf("clean run logged a warning it should not have: %q", got)
	}

	repo2 := newFakeRepo()
	seedSyntaxa(repo2, "CA01A")
	var dirty bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&dirty, nil)))
	dist2, cov2 := writeDistFiles(t,
		minimalDistribution+
			"CI01E,evc_territory,spain-atlantic,verified\n"+
			"CA01A,evc_territory,romania,1\n",
		minimalCoverage)
	rep, err := IngestSyntaxonDistribution(context.Background(), repo2, dist2, cov2)
	slog.SetDefault(prev)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution (dirty): %v", err)
	}
	if len(rep.UnknownSyntaxa) == 0 || rep.SkippedRows == 0 {
		t.Fatalf("Report = %+v, wollte sowohl UnknownSyntaxa als auch SkippedRows > 0", rep)
	}
	if got := dirty.String(); !strings.Contains(got, "does not carry") {
		t.Errorf("dirty run log = %q, want it to warn about the unknown syntaxon", got)
	}
	if got := dirty.String(); !strings.Contains(got, "skipped malformed rows") {
		t.Errorf("dirty run log = %q, want it to warn about the skipped row", got)
	}
}

func TestIngestSyntaxonDistributionWarntNurBeiFehlenderDatei(t *testing.T) {
	// Distribution is extra information, exactly like IngestDistribution's —
	// unlike the hierarchy in Teilprojekt A, which is a primary source. A
	// missing file must not abort a five-minute ingest.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dir := t.TempDir()
	rep, err := IngestSyntaxonDistribution(context.Background(), repo,
		filepath.Join(dir, "syntaxon_distribution.csv"),
		filepath.Join(dir, "syntaxon_distribution_coverage.csv"))
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if !reflect.DeepEqual(rep, SyntaxonDistributionReport{}) {
		t.Errorf("Report = %+v, erwartet den Nullwert", rep)
	}
}

func TestIngestSyntaxonDistributionBrichtBeiFehlenderCoverageDateiAb(t *testing.T) {
	// The distribution file WITHOUT the coverage file is the one combination
	// that must not pass: every row would be written and every alliance's
	// empty cell would then read as "nobody looked" instead of "does not
	// occur". Half the truth is worse here than none.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")
	dist, cov := writeDistFiles(t, minimalDistribution, "")

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil {
		t.Fatal("IngestSyntaxonDistribution lief ohne die Coverage-Datei durch")
	}
	if !strings.Contains(err.Error(), "syntaxon_distribution_coverage.csv") {
		t.Errorf("Fehler benennt die Datei nicht: %v", err)
	}
	_ = cov
}

func TestIngestSyntaxonDistributionUeberspringtFremdesSchema(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc-territory,austria-alps,verified\n"+
			"CA01A,evc_territory,albania,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 1 || rep.SkippedRows != 1 {
		t.Errorf("Report = %+v, erwartet Written 1 / SkippedRows 1", rep)
	}
}

// TestIngestSyntaxonDistributionUeberspringtWgsrpdSchema covers Befund 4: a
// scheme domain.IsKnownAreaScheme accepts (wgsrpd_l3) is still wrong here —
// syntaxa distribution is read exclusively under evc_territory
// (SyntaxonDistribution, SyntaxonOccurrencesInArea), so a wgsrpd_l3 row must
// be skipped and counted, not written to a place no read path ever looks.
func TestIngestSyntaxonDistributionUeberspringtWgsrpdSchema(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,wgsrpd_l3,GER,verified\n",
		minimalCoverage)

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 0 || rep.SkippedRows != 1 {
		t.Errorf("Report = %+v, erwartet Written 0 / SkippedRows 1", rep)
	}
}

// TestIngestSyntaxonDistributionUeberspringtCoverageMitWgsrpdSchema is the
// coverage-reader counterpart of the test above.
func TestIngestSyntaxonDistributionUeberspringtCoverageMitWgsrpdSchema(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t, minimalDistribution,
		"syntaxon_id,area_scheme\nCA01A,wgsrpd_l3\n")

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Covered != 0 || rep.SkippedRows != 1 {
		t.Errorf("Report = %+v, erwartet Covered 0 / SkippedRows 1", rep)
	}
}

func TestIngestSyntaxonDistributionUeberspringtUnbekannteAuspraegung(t *testing.T) {
	// The pipeline aborts on an unknown cell value, so this row should not
	// exist. It is skipped rather than written because the CHECK would reject
	// it anyway and a failed transaction would lose the other 11527 rows.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			"CA01A,evc_territory,austria-alps,1\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 0 || rep.SkippedRows != 1 {
		t.Errorf("Report = %+v, erwartet Written 0 / SkippedRows 1", rep)
	}
}

func TestIngestSyntaxonDistributionUeberspringtUnvollstaendigeZeile(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code,occurrence\n"+
			",evc_territory,austria-alps,verified\n"+
			"CA01A,evc_territory,,verified\n",
		"syntaxon_id,area_scheme\nCA01A,evc_territory\n")

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Written != 0 || rep.SkippedRows != 2 {
		t.Errorf("Report = %+v, erwartet Written 0 / SkippedRows 2", rep)
	}
}

func TestIngestSyntaxonDistributionMeldetStatFehlerJenseitsVonFehlt(t *testing.T) {
	// A regular file where a directory is expected turns os.Stat's error into
	// something other than IsNotExist (ENOTDIR) -- that branch must surface as
	// an error, not be swallowed as "file missing".
	repo := newFakeRepo()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("Schreiben von %s: %v", blocker, err)
	}
	csvPath := filepath.Join(blocker, "syntaxon_distribution.csv")

	_, err := IngestSyntaxonDistribution(context.Background(), repo, csvPath, filepath.Join(dir, "cov.csv"))
	if err == nil {
		t.Fatal("erwartet einen Fehler, csvPath liegt unter einer Datei statt einem Verzeichnis")
	}
	if !strings.Contains(err.Error(), "statting") {
		t.Errorf("Fehler benennt das Stat nicht: %v", err)
	}
}

func TestIngestSyntaxonDistributionGibtAllSyntaxaFehlerWeiter(t *testing.T) {
	repo := newFakeRepo()
	repo.allSyntaxaErr = errBoom
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil || !strings.Contains(err.Error(), "listing syntaxa") {
		t.Errorf("Fehler = %v, erwartet einen Fehler ueber AllSyntaxa", err)
	}
}

func TestIngestSyntaxonDistributionGibtBeginFehlerWeiter(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	repo.beginErr = errBoom
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil {
		t.Fatal("erwartet einen Fehler, Begin schlaegt fehl")
	}
}

func TestIngestSyntaxonDistributionRolltBeiSchreibfehlerZurueck(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")
	repo.failOn = "UpsertSyntaxonDistribution"
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil {
		t.Fatal("erwartet einen Fehler, UpsertSyntaxonDistribution schlaegt fehl")
	}
	if !repo.rolledBack {
		t.Error("erwartet ein Rollback nach dem Schreibfehler")
	}
	if repo.committed {
		t.Error("commit haette nach dem Schreibfehler nicht laufen duerfen")
	}
}

func TestIngestSyntaxonDistributionMeldetFehlgeschlagenesRollback(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	repo.failOn = "UpsertSyntaxonDistribution"
	repo.rollbackErr = errBoom
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil || !strings.Contains(err.Error(), "rollback also failed") {
		t.Errorf("Fehler = %v, erwartet einen Hinweis auf das fehlgeschlagene Rollback", err)
	}
}

func TestIngestSyntaxonDistributionGibtCommitFehlerWeiter(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")
	repo.commitErr = errBoom
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil || !strings.Contains(err.Error(), "committing syntaxon distribution transaction") {
		t.Errorf("Fehler = %v, erwartet einen Commit-Fehler", err)
	}
}

func TestIngestSyntaxonDistributionGibtLesefehlerDerVerbreitungsdateiWeiter(t *testing.T) {
	// A distribution file missing a required column is a hard reader error
	// (readAll fails the whole read), distinct from a malformed row that is
	// merely skipped.
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t,
		"syntaxon_id,area_scheme,area_code\nCA01A,evc_territory,austria-alps\n",
		minimalCoverage)

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil || !strings.Contains(err.Error(), "missing required column") {
		t.Errorf("Fehler = %v, erwartet einen Header-Fehler", err)
	}
}

func TestIngestSyntaxonDistributionGibtLesefehlerDerCoverageDateiWeiter(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t, minimalDistribution, "syntaxon_id\nCA01A\n")

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil || !strings.Contains(err.Error(), "missing required column") {
		t.Errorf("Fehler = %v, erwartet einen Header-Fehler", err)
	}
}

func TestIngestSyntaxonDistributionUeberspringtUnvollstaendigeCoverageZeile(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t, minimalDistribution,
		"syntaxon_id,area_scheme\n,evc_territory\nCA01A,\n")

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Covered != 0 || rep.SkippedRows != 2 {
		t.Errorf("Report = %+v, erwartet Covered 0 / SkippedRows 2", rep)
	}
}

func TestIngestSyntaxonDistributionUeberspringtCoverageMitFremdemSchema(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	dist, cov := writeDistFiles(t, minimalDistribution,
		"syntaxon_id,area_scheme\nCA01A,evc-territory\n")

	rep, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err != nil {
		t.Fatalf("IngestSyntaxonDistribution: %v", err)
	}
	if rep.Covered != 0 || rep.SkippedRows != 1 {
		t.Errorf("Report = %+v, erwartet Covered 0 / SkippedRows 1", rep)
	}
}

func TestIngestSyntaxonDistributionRolltBeiCoverageSchreibfehlerZurueck(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A")
	repo.failOn = "UpsertSyntaxonDistributionCoverage"
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil {
		t.Fatal("erwartet einen Fehler, UpsertSyntaxonDistributionCoverage schlaegt fehl")
	}
	if !repo.rolledBack {
		t.Error("erwartet ein Rollback nach dem Coverage-Schreibfehler")
	}
}

// The following exercise the five fakeRepo methods Task 5 added mechanically
// (UpsertSyntaxonDistribution, UpsertSyntaxonDistributionCoverage,
// SyntaxonDistribution, SyntaxonOccurrencesInArea, SyntaxaWithCoverage)
// against the port comments directly, calling the Repository methods rather
// than reaching into fakeRepo's fields — Task 6's brief calls their behavior
// an untested interpretation and asks to check it, not trust it.

func TestFakeRepoUpsertSyntaxonDistributionIstIdempotentUndUeberschreibt(t *testing.T) {
	repo := newFakeRepo()
	tx, err := repo.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSyntaxonDistribution("CA01A", "evc_territory", "austria-alps", domain.OccurrenceUncertain); err != nil {
		t.Fatalf("erster Upsert: %v", err)
	}
	// A repinned source may upgrade uncertain to verified; the port comment
	// promises the fake overwrites rather than keeping the stale value.
	if err := tx.UpsertSyntaxonDistribution("CA01A", "evc_territory", "austria-alps", domain.OccurrenceVerified); err != nil {
		t.Fatalf("zweiter Upsert: %v", err)
	}

	dist, err := repo.SyntaxonDistribution(context.Background(), "CA01A", "evc_territory")
	if err != nil {
		t.Fatalf("SyntaxonDistribution: %v", err)
	}
	if len(dist.Verified) != 1 || dist.Verified[0] != "austria-alps" {
		t.Errorf("Verified = %v, erwartet [austria-alps]", dist.Verified)
	}
	if len(dist.Uncertain) != 0 {
		t.Errorf("Uncertain = %v, erwartet leer — der zweite Upsert haette den ersten ersetzen muessen, nicht ergaenzen", dist.Uncertain)
	}
}

func TestFakeRepoUpsertSyntaxonDistributionCoverageIstIdempotent(t *testing.T) {
	repo := newFakeRepo()
	tx, err := repo.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for range 2 {
		if err := tx.UpsertSyntaxonDistributionCoverage("CA01A", "evc_territory"); err != nil {
			t.Fatalf("UpsertSyntaxonDistributionCoverage: %v", err)
		}
	}

	covered, err := repo.SyntaxaWithCoverage(context.Background(), "evc_territory")
	if err != nil {
		t.Fatalf("SyntaxaWithCoverage: %v", err)
	}
	if len(covered) != 1 || !covered["CA01A"] {
		t.Errorf("SyntaxaWithCoverage = %v, erwartet genau {CA01A: true} — ein zweiter Aufruf haette keine Dopplung anlegen duerfen", covered)
	}
}

func TestFakeRepoSyntaxonDistributionUnterscheidetCoveredVonUnbekannt(t *testing.T) {
	// CA01A: coverage row, no occurrence -> Covered true, both lists empty.
	// CA01B: no rows at all -> also Covered false, both lists empty, but the
	// caller cannot tell the two apart from this struct alone — that is why
	// SyntaxaWithCoverage exists as a separate query.
	repo := newFakeRepo()
	tx, err := repo.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSyntaxonDistributionCoverage("CA01A", "evc_territory"); err != nil {
		t.Fatalf("UpsertSyntaxonDistributionCoverage: %v", err)
	}

	covered, err := repo.SyntaxonDistribution(context.Background(), "CA01A", "evc_territory")
	if err != nil {
		t.Fatalf("SyntaxonDistribution(CA01A): %v", err)
	}
	if !covered.Covered || len(covered.Verified) != 0 || len(covered.Uncertain) != 0 {
		t.Errorf("CA01A = %+v, erwartet Covered true mit leeren Listen", covered)
	}

	uncovered, err := repo.SyntaxonDistribution(context.Background(), "CA01B", "evc_territory")
	if err != nil {
		t.Fatalf("SyntaxonDistribution(CA01B): %v", err)
	}
	if uncovered.Covered {
		t.Errorf("CA01B = %+v, erwartet Covered false — kein Coverage-Aufruf fuer dieses Syntaxon", uncovered)
	}
}

func TestFakeRepoSyntaxonOccurrencesInAreaFiltertNachGebiet(t *testing.T) {
	repo := newFakeRepo()
	tx, err := repo.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertSyntaxonDistribution("CA01A", "evc_territory", "austria-alps", domain.OccurrenceVerified); err != nil {
		t.Fatalf("Upsert CA01A: %v", err)
	}
	if err := tx.UpsertSyntaxonDistribution("CA01B", "evc_territory", "austria-alps", domain.OccurrenceUncertain); err != nil {
		t.Fatalf("Upsert CA01B: %v", err)
	}
	if err := tx.UpsertSyntaxonDistribution("CA01A", "evc_territory", "czech-republic", domain.OccurrenceVerified); err != nil {
		t.Fatalf("Upsert CA01A/czech-republic: %v", err)
	}

	occ, err := repo.SyntaxonOccurrencesInArea(context.Background(), "evc_territory", "austria-alps")
	if err != nil {
		t.Fatalf("SyntaxonOccurrencesInArea: %v", err)
	}
	if len(occ) != 2 || occ["CA01A"] != domain.OccurrenceVerified || occ["CA01B"] != domain.OccurrenceUncertain {
		t.Errorf("SyntaxonOccurrencesInArea(austria-alps) = %v, erwartet CA01A verified / CA01B uncertain, kein czech-republic-Eintrag", occ)
	}
}

func TestIngestSyntaxonDistributionMeldetFehlerBeimLeeren(t *testing.T) {
	repo := newFakeRepo()
	seedSyntaxa(repo, "CA01A", "CA01B")
	repo.failOn = "ClearSyntaxonDistribution"
	dist, cov := writeDistFiles(t, minimalDistribution, minimalCoverage)

	_, err := IngestSyntaxonDistribution(context.Background(), repo, dist, cov)
	if err == nil {
		t.Fatal("IngestSyntaxonDistribution lief trotz fehlschlagendem ClearSyntaxonDistribution durch")
	}
	if !strings.Contains(err.Error(), "clearing syntaxon distribution before ingest") {
		t.Errorf("Fehler = %v, erwartet den Leer-Kontext", err)
	}
	if !repo.rolledBack {
		t.Error("erwartet ein Rollback nach dem fehlgeschlagenen Leeren")
	}
}
