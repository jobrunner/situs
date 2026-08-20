package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

func writeCSV(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// seedDir writes a minimal but complete CSV set: one EUNIS type, one annex1
// type, and a '=' crosswalk between them.
func seedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeCSV(t, dir, "typologies.csv",
		"id,scheme,version,name,source_ref\n"+
			"eunis@2021,eunis,2021,EUNIS 2021,https://example.org/eunis\n"+
			"annex1,annex1,,Habitats Directive Annex I,https://example.org/annex1\n")
	writeCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\n"+
			"eunis@2021,R22,3,Hay meadow,R2,\n"+
			"annex1,6510,,Lowland hay meadows,,0\n")
	writeCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n"+
			"eunis@2021,R22,annex1,6510,=\n")
	writeCSV(t, dir, "syntaxa.csv",
		"id,rank,name,parent_id\nARR,alliance,Arrhenatherion elatioris,MOL\n")
	writeCSV(t, dir, "habitat_type_syntaxa.csv",
		"typology_id,code,syntaxon_id\neunis@2021,R22,ARR\n")
	return dir
}

func TestIngestCSV_LoadsEverySource(t *testing.T) {
	repo := newFakeRepo()
	rep, err := IngestCSV(context.Background(), repo, seedDir(t))
	if err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if rep.HabitatTypes != 2 {
		t.Errorf("HabitatTypes = %d, want 2", rep.HabitatTypes)
	}
	if rep.Crosswalks != 1 || rep.SyntaxonLinks != 1 {
		t.Errorf("Crosswalks/SyntaxonLinks = %d/%d, want 1/1", rep.Crosswalks, rep.SyntaxonLinks)
	}
	if !repo.committed {
		t.Error("ingest did not commit")
	}
}

// The version crosswalk and the annex1 crosswalk share one table — an ingest
// that special-cases annex1 would break this.
func TestIngestCSV_AnnexOneUsesTheSameCrosswalkTable(t *testing.T) {
	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, seedDir(t)); err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	want := domain.Crosswalk{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}
	if len(repo.crosswalks) != 1 || repo.crosswalks[0] != want {
		t.Errorf("crosswalks = %+v, want exactly [%+v]", repo.crosswalks, want)
	}
}

// A malformed row must not abort the whole ingest, but it must be counted —
// silent skipping is how coverage gaps hide.
func TestIngestCSV_CountsSkippedRowsInsteadOfFailing(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n"+
			"eunis@2021,R22,annex1,6510,=\n"+
			"eunis@2021,R23,annex1,6520,~\n") // '~' is not a valid qualifier

	repo := newFakeRepo()
	rep, err := IngestCSV(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if rep.Crosswalks != 1 {
		t.Errorf("Crosswalks = %d, want 1 (the valid row)", rep.Crosswalks)
	}
	if rep.SkippedRows != 1 {
		t.Errorf("SkippedRows = %d, want 1 (the bad qualifier)", rep.SkippedRows)
	}
}

// A short row (fewer fields than the header) is the shape a truncated,
// hand-edited CSV row actually takes. It must be counted and skipped, not
// abort the whole file — this is the scenario the CSV-syntax test below
// (an unterminated quote) does not cover.
func TestIngestCSV_SkipsShortRowInsteadOfAbortingTheFile(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\n"+
			"eunis@2021,R22,3,Hay meadow,R2,\n"+
			"annex1,6510,,Lowland hay meadows,,0\n"+
			"eunis@2021,R23,3,Truncated\n") // only 4 of 6 fields

	repo := newFakeRepo()
	rep, err := IngestCSV(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if rep.HabitatTypes != 2 {
		t.Errorf("HabitatTypes = %d, want 2 (the two well-formed rows)", rep.HabitatTypes)
	}
	if rep.SkippedRows != 1 {
		t.Errorf("SkippedRows = %d, want 1 (the short row)", rep.SkippedRows)
	}
}

// A canceled context must stop the ingest before any file is committed —
// otherwise a caller has no way to abandon a run mid-flight.
func TestIngestCSV_CanceledContextStopsBeforeCommitting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := newFakeRepo()
	if _, err := IngestCSV(ctx, repo, seedDir(t)); err == nil {
		t.Fatal("IngestCSV with a canceled context = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a canceled context")
	}
	if !repo.rolledBack {
		t.Error("ingest did not roll back after a canceled context")
	}
	if len(repo.typologies) != 0 {
		t.Errorf("typologies = %v, want none read after cancellation", repo.typologies)
	}
}

// "Logged with file and line" is a brief requirement a coverage count alone
// does not verify — this pins the actual record content.
func TestIngestCSV_SkipWarningNamesFileAndLine(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n"+
			"eunis@2021,R22,annex1,6510,=\n"+
			"eunis@2021,R23,annex1,6520,~\n")

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "file=crosswalks.csv") {
		t.Errorf("log = %q, want it to name the file", got)
	}
	if !strings.Contains(got, "line=3") {
		t.Errorf("log = %q, want it to name the line", got)
	}
}

func TestIngestCSV_MissingFileFails(t *testing.T) {
	dir := seedDir(t)
	if err := os.Remove(filepath.Join(dir, "syntaxa.csv")); err != nil {
		t.Fatalf("removing syntaxa.csv: %v", err)
	}

	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, dir); err == nil {
		t.Fatal("IngestCSV with a missing source file = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a missing source file")
	}
	if !repo.rolledBack {
		t.Error("ingest did not roll back after a missing source file")
	}
}

func TestIngestCSV_MissingRequiredColumnFails(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "crosswalks.csv", "from_typology,from_code,to_typology,to_code\n"+
		"eunis@2021,R22,annex1,6510\n") // no "qualifier" column

	repo := newFakeRepo()
	_, err := IngestCSV(context.Background(), repo, dir)
	if err == nil {
		t.Fatal("IngestCSV with a missing required column = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "qualifier") {
		t.Errorf("error = %q, want it to name the missing column", err)
	}
}

func TestIngestCSV_SkipsMalformedLevelAndPriority(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\n"+
			"eunis@2021,R22,3,Hay meadow,R2,\n"+
			"annex1,6510,,Lowland hay meadows,,0\n"+
			"eunis@2021,R23,not-a-number,Bad level,R2,\n"+
			"annex1,6520,,Bad priority,,not-a-bool\n")

	repo := newFakeRepo()
	rep, err := IngestCSV(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if rep.HabitatTypes != 2 {
		t.Errorf("HabitatTypes = %d, want 2 (the two well-formed rows)", rep.HabitatTypes)
	}
	if rep.SkippedRows != 2 {
		t.Errorf("SkippedRows = %d, want 2 (bad level + bad priority)", rep.SkippedRows)
	}
}

func TestIngestCSV_SkipsMalformedSyntaxonLinkTypology(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "habitat_type_syntaxa.csv",
		"typology_id,code,syntaxon_id\neunis@2021,R22,ARR\n,R99,ARR\n")

	repo := newFakeRepo()
	rep, err := IngestCSV(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if rep.SyntaxonLinks != 1 {
		t.Errorf("SyntaxonLinks = %d, want 1", rep.SyntaxonLinks)
	}
	if rep.SkippedRows != 1 {
		t.Errorf("SkippedRows = %d, want 1 (empty typology id)", rep.SkippedRows)
	}
}

func TestIngestCSV_RepositoryErrorRollsBackAndReturnsTheError(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertHabitatType"

	_, err := IngestCSV(context.Background(), repo, seedDir(t))
	if err == nil {
		t.Fatal("IngestCSV with a repository error = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a repository error")
	}
	if !repo.rolledBack {
		t.Error("ingest did not roll back after a repository error")
	}
}

func TestIngestCSV_BeginErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.beginErr = fmt.Errorf("boom")

	if _, err := IngestCSV(context.Background(), repo, seedDir(t)); err == nil {
		t.Fatal("IngestCSV with a Begin error = nil error, want an error")
	}
}

func TestIngestCSV_CommitErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.commitErr = fmt.Errorf("disk full")

	if _, err := IngestCSV(context.Background(), repo, seedDir(t)); err == nil {
		t.Fatal("IngestCSV with a Commit error = nil error, want an error")
	}
}

func TestIngestCSV_RollbackErrorIsWrappedWithTheOriginal(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = "UpsertHabitatType"
	repo.rollbackErr = fmt.Errorf("connection lost")

	_, err := IngestCSV(context.Background(), repo, seedDir(t))
	if err == nil {
		t.Fatal("IngestCSV = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error = %q, want it to mention the rollback failure too", err)
	}
}

func TestIngestCSV_MissingDirectoryFails(t *testing.T) {
	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("IngestCSV(missing dir) = nil error, want an error")
	}
}

func TestIngestCSV_EmptyFileFailsOnTheHeader(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "syntaxa.csv", "")

	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, dir); err == nil {
		t.Fatal("IngestCSV(empty syntaxa.csv) = nil error, want an error")
	}
}

func TestIngestCSV_MalformedCSVSyntaxFails(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "syntaxa.csv",
		"id,rank,name,parent_id\nARR,alliance,\"unterminated,MOL\n")

	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, dir); err == nil {
		t.Fatal("IngestCSV(malformed csv syntax) = nil error, want an error")
	}
}

func TestIngestCSV_MalformedTypologyRowAbortsTheIngest(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "typologies.csv",
		"id,scheme,version,name,source_ref\nnot valid @@ id,eunis,2021,Broken,https://example.org\n")

	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, dir); err == nil {
		t.Fatal("IngestCSV(malformed typology id) = nil error, want an error")
	}
}

func TestIngestCSV_SkipsMalformedHabitatTypeTypology(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\n"+
			"eunis@2021,R22,3,Hay meadow,R2,\n"+
			",R99,3,Orphaned,R2,\n")

	repo := newFakeRepo()
	rep, err := IngestCSV(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if rep.SkippedRows != 1 {
		t.Errorf("SkippedRows = %d, want 1 (empty typology id)", rep.SkippedRows)
	}
}

func TestIngestCSV_SkipsMalformedCrosswalkTypologies(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n"+
			"eunis@2021,R22,annex1,6510,=\n"+
			",R23,annex1,6520,=\n"+
			"eunis@2021,R24,,6530,=\n")

	repo := newFakeRepo()
	rep, err := IngestCSV(context.Background(), repo, dir)
	if err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if rep.Crosswalks != 1 {
		t.Errorf("Crosswalks = %d, want 1", rep.Crosswalks)
	}
	if rep.SkippedRows != 2 {
		t.Errorf("SkippedRows = %d, want 2 (empty from/to typology)", rep.SkippedRows)
	}
}

func TestIngestCSV_RepositoryErrorPerEntity(t *testing.T) {
	for _, failOn := range []string{"UpsertTypology", "UpsertHabitatType", "UpsertCrosswalk", "UpsertSyntaxon", "LinkSyntaxon"} {
		t.Run(failOn, func(t *testing.T) {
			repo := newFakeRepo()
			repo.failOn = failOn

			if _, err := IngestCSV(context.Background(), repo, seedDir(t)); err == nil {
				t.Fatalf("IngestCSV with %s failing = nil error, want an error", failOn)
			}
			if repo.committed {
				t.Errorf("ingest committed despite %s failing", failOn)
			}
			if !repo.rolledBack {
				t.Errorf("ingest did not roll back after %s failed", failOn)
			}
		})
	}
}

// fakeRepo is a growable double for output.Repository: each entity type gets
// its own slice so later tasks (species roles, localizations) can extend it
// without reshaping what is here.
type fakeRepo struct {
	typologies   []domain.Typology
	types        []domain.HabitatType
	crosswalks   []domain.Crosswalk
	syntaxa      []domain.Syntaxon
	syntaxaLinks []struct {
		key        domain.HabitatTypeKey
		syntaxonID string
	}
	speciesRoles  []domain.SpeciesRole
	localizations []domain.Localization
	distribution  []fakeDistribution
	committed     bool
	rolledBack    bool

	beginErr        error
	commitErr       error
	rollbackErr     error
	crosswalksToErr error
	// The read side's injectable failures (Task 8): each fails exactly one
	// Repository read so a query test can pin that the failure surfaces.
	typologyErr     error
	habitatTypeErr  error
	crosswalksErr   error
	speciesRolesErr error
	syntaxonErr     error
	syntaxaErr      error
	syntaxonKeysErr error
	// localizationErr fails every Localization call; localizationErrOnCall,
	// if non-zero, instead fails only the n-th call (1-indexed) — needed to
	// exercise DeriveGermanLabels' second Localization lookup (the Annex I
	// target) without also failing the first (the source check).
	localizationErr       error
	localizationErrOnCall int
	localizationCalls     int
	// failOn names an Upsert*/LinkSyntaxon method that should fail once
	// called, to exercise the rollback path.
	failOn string
	// areasErr fails AreasForConcepts and KnownAreaCodes.
	areasErr error
}

// fakeDistribution is one recorded UpsertDistribution call.
type fakeDistribution struct {
	ConceptID string
	Area      domain.Area
}

func newFakeRepo() *fakeRepo { return &fakeRepo{} }

func (r *fakeRepo) Begin(context.Context) (output.IngestTx, error) {
	if r.beginErr != nil {
		return nil, r.beginErr
	}
	return r, nil
}

func (r *fakeRepo) failIfNamed(name string) error {
	if r.failOn == name {
		return fmt.Errorf("fakeRepo: injected failure in %s", name)
	}
	return nil
}

func (r *fakeRepo) HabitatType(_ context.Context, key domain.HabitatTypeKey) (domain.HabitatType, error) {
	if r.habitatTypeErr != nil {
		return domain.HabitatType{}, r.habitatTypeErr
	}
	for _, h := range r.types {
		if h.Key == key {
			return h, nil
		}
	}
	return domain.HabitatType{}, output.ErrNotFound
}

func (r *fakeRepo) UpsertTypology(t domain.Typology) error {
	if err := r.failIfNamed("UpsertTypology"); err != nil {
		return err
	}
	r.typologies = append(r.typologies, t)
	return nil
}

func (r *fakeRepo) UpsertHabitatType(h domain.HabitatType) error {
	if err := r.failIfNamed("UpsertHabitatType"); err != nil {
		return err
	}
	r.types = append(r.types, h)
	return nil
}

func (r *fakeRepo) UpsertCrosswalk(c domain.Crosswalk) error {
	if err := r.failIfNamed("UpsertCrosswalk"); err != nil {
		return err
	}
	r.crosswalks = append(r.crosswalks, c)
	return nil
}

func (r *fakeRepo) UpsertSyntaxon(s domain.Syntaxon) error {
	if err := r.failIfNamed("UpsertSyntaxon"); err != nil {
		return err
	}
	r.syntaxa = append(r.syntaxa, s)
	return nil
}

func (r *fakeRepo) LinkSyntaxon(key domain.HabitatTypeKey, syntaxonID string) error {
	if err := r.failIfNamed("LinkSyntaxon"); err != nil {
		return err
	}
	r.syntaxaLinks = append(r.syntaxaLinks, struct {
		key        domain.HabitatTypeKey
		syntaxonID string
	}{key, syntaxonID})
	return nil
}

func (r *fakeRepo) UpsertSpeciesRole(role domain.SpeciesRole) error {
	if err := r.failIfNamed("UpsertSpeciesRole"); err != nil {
		return err
	}
	r.speciesRoles = append(r.speciesRoles, role)
	return nil
}

func (r *fakeRepo) UpsertLocalization(l domain.Localization) error {
	if err := r.failIfNamed("UpsertLocalization"); err != nil {
		return err
	}
	r.localizations = append(r.localizations, l)
	return nil
}

func (r *fakeRepo) UpsertDistribution(conceptID string, a domain.Area) error {
	if r.failOn == "UpsertDistribution" {
		return errors.New("boom")
	}
	r.distribution = append(r.distribution, fakeDistribution{ConceptID: conceptID, Area: a})
	return nil
}

func (r *fakeRepo) AreasForConcepts(_ context.Context, conceptIDs []string, scheme string) (map[string][]string, error) {
	if r.areasErr != nil {
		return nil, r.areasErr
	}
	out := map[string][]string{}
	for _, id := range conceptIDs {
		for _, d := range r.distribution {
			if d.ConceptID == id && d.Area.Scheme == scheme {
				out[id] = append(out[id], d.Area.Code)
			}
		}
	}
	return out, nil
}

func (r *fakeRepo) KnownAreaCodes(_ context.Context, scheme string) ([]string, error) {
	if r.areasErr != nil {
		return nil, r.areasErr
	}
	seen := map[string]bool{}
	out := []string{}
	for _, d := range r.distribution {
		if d.Area.Scheme == scheme && !seen[d.Area.Code] {
			seen[d.Area.Code] = true
			out = append(out, d.Area.Code)
		}
	}
	return out, nil
}

func (r *fakeRepo) CrosswalksTo(_ context.Context, typology domain.TypologyID) ([]domain.Crosswalk, error) {
	if r.crosswalksToErr != nil {
		return nil, r.crosswalksToErr
	}
	var out []domain.Crosswalk
	for _, c := range r.crosswalks {
		if c.To.Typology == typology {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *fakeRepo) Localization(_ context.Context, entityType, entityKey, lang, field string) ([]domain.Localization, error) {
	r.localizationCalls++
	if r.localizationErr != nil {
		return nil, r.localizationErr
	}
	if r.localizationErrOnCall != 0 && r.localizationCalls == r.localizationErrOnCall {
		return nil, fmt.Errorf("fakeRepo: injected Localization failure on call %d", r.localizationCalls)
	}
	var out []domain.Localization
	for _, l := range r.localizations {
		if l.EntityType == entityType && l.EntityKey == entityKey && l.Lang == lang && l.Field == field {
			out = append(out, l)
		}
	}
	return out, nil
}

// derivedFor returns the derived localization for entityKey, for tests that
// assert on the one row DeriveGermanLabels produced.
func (r *fakeRepo) derivedFor(entityType, entityKey string) domain.Localization {
	return r.localizationFor(entityType, entityKey, "derived-annex1")
}

// localizationFor returns the localization matching entityKey and source, for
// tests that assert an existing row was (or was not) touched.
func (r *fakeRepo) localizationFor(entityType, entityKey, source string) domain.Localization {
	for _, l := range r.localizations {
		if l.EntityType == entityType && l.EntityKey == entityKey && l.Source == source {
			return l
		}
	}
	return domain.Localization{}
}

func (r *fakeRepo) Commit() error {
	if r.commitErr != nil {
		return r.commitErr
	}
	r.committed = true
	return nil
}

func (r *fakeRepo) Rollback() error {
	r.rolledBack = true
	return r.rollbackErr
}
