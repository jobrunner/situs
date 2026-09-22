package application

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
	return dir
}

// The syntaxa CSVs are still present in the pipeline's output directory
// (other tools read them), but writing vegetation units and their edges is
// IngestSyntaxa's job now (Task 5-7) — IngestCSV must leave them alone.
func TestIngestCSVSchreibtKeineSyntaxaMehr(t *testing.T) {
	repo := newFakeRepo()
	dir := t.TempDir()
	writeCSV(t, dir, "typologies.csv", "id,scheme,version,name,source_ref\neunis@2021,eunis,2021,EUNIS,\n")
	writeCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\neunis@2021,T1,1,Wald,,\n")
	writeCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n")
	// The files are present but must no longer be read by IngestCSV.
	writeCSV(t, dir, "syntaxa.csv", "id,rank,name,parent_id\nX-01A,alliance,Darf nicht rein,\n")
	writeCSV(t, dir, "habitat_type_syntaxa.csv", "typology_id,code,syntaxon_id\neunis@2021,T1,X-01A\n")

	if _, err := IngestCSV(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}
	if repo.has("X-01A") {
		t.Error("IngestCSV hat ein Syntaxon geschrieben")
	}
	if len(repo.syntaxaLinks) != 0 {
		t.Errorf("IngestCSV hat %d Kanten geschrieben", len(repo.syntaxaLinks))
	}
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
	if rep.Crosswalks != 1 {
		t.Errorf("Crosswalks = %d, want 1", rep.Crosswalks)
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
	if err := os.Remove(filepath.Join(dir, "crosswalks.csv")); err != nil {
		t.Fatalf("removing crosswalks.csv: %v", err)
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

// Priority is a tri-state: an empty column means "not stated", and 0 and 1 are
// both statements. Reading 0 as true would mark every ordinary Annex I type as
// a priority habitat.
func TestIngestCSV_ReadsThePriorityColumnAsAThreeStateFlag(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\n"+
			"annex1,6510,,Lowland hay meadows,,0\n"+
			"annex1,6210,,Semi-natural dry grasslands,,1\n"+
			"annex1,6520,,Mountain hay meadows,,\n")

	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, dir); err != nil {
		t.Fatalf("IngestCSV: %v", err)
	}

	got := make(map[string]*bool, len(repo.types))
	for _, h := range repo.types {
		got[h.Key.Code] = h.Priority
	}
	yes, no := true, false
	for code, want := range map[string]*bool{"6510": &no, "6210": &yes, "6520": nil} {
		switch {
		case want == nil && got[code] != nil:
			t.Errorf("priority of %s = %t, want unstated (nil)", code, *got[code])
		case want != nil && got[code] == nil:
			t.Errorf("priority of %s = nil, want %t", code, *want)
		case want != nil && *got[code] != *want:
			t.Errorf("priority of %s = %t, want %t", code, *got[code], *want)
		}
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
	writeCSV(t, dir, "crosswalks.csv", "")

	repo := newFakeRepo()
	if _, err := IngestCSV(context.Background(), repo, dir); err == nil {
		t.Fatal("IngestCSV(empty crosswalks.csv) = nil error, want an error")
	}
}

func TestIngestCSV_MalformedCSVSyntaxFails(t *testing.T) {
	dir := seedDir(t)
	writeCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\neunis@2021,R22,\"unterminated,6510,=\n")

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
	for _, failOn := range []string{"UpsertTypology", "UpsertHabitatType", "UpsertCrosswalk"} {
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
	allSyntaxaErr error
	// eeaCodesErr fails SyntaxonIDsByEEACode, exercising IngestSyntaxa's
	// pre-Begin read of the index's existing state (Task 7).
	eeaCodesErr   error
	speciesRoles  []domain.SpeciesRole
	speciesNames  []domain.SpeciesName
	searchErr     error
	localizations []domain.Localization
	distribution  []fakeDistribution
	areas         []domain.NamedArea
	descriptions  []domain.HabitatDescription
	namedAreas    []domain.NamedArea
	traitValues   []fakeTraitValue
	traitVocabs   []fakeTraitVocab
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
	// syntaxonByEEACodeErr fails only SyntaxonByEEACode, exercising
	// syntaxonByIDOrEEACode's fallback-lookup error path independently of
	// syntaxonErr, which fails the primary Syntaxon lookup instead.
	syntaxonByEEACodeErr error
	syntaxaErr           error
	syntaxonKeysErr      error
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
	// conceptIDsErr fails ConceptIDs, exercising IngestDistribution's error path.
	conceptIDsErr error
	// knownVocabsErr fails KnownVocabs, exercising QueryService.Traits' ?vocab=
	// validation error path.
	knownVocabsErr error
	// traitsErr fails Traits, exercising QueryService.Traits' data-fetch error path.
	traitsErr error
	// traitsForConceptsErr fails TraitsForConcepts.
	traitsForConceptsErr error
	// descriptionErr fails Description, exercising the detail route's
	// description error path.
	descriptionErr error
	// The navigation failures (subproject B): each fails exactly one of the
	// new reads, so a use-case test can pin that the failure surfaces.
	syntaxonChildrenErr  error
	syntaxonAncestorsErr error
	habitatTypeCountErr  error
	syntaxaByRankErr     error
	syntaxonRanksErr     error
	// syntaxonDistributionErr fails the read-side SyntaxonDistribution call
	// (subproject C, Task 8), exercising Syntaxon's distribution error path.
	syntaxonDistributionErr error
	// syntaxonOccurrencesInAreaErr and syntaxaWithCoverageErr fail the two
	// reads SyntaxaByRank's ?area= filter needs (subproject C, Task 9),
	// exercising syntaxonAreaLookup's two remaining error paths (KnownAreaCodes'
	// is exercised through areasErr already).
	syntaxonOccurrencesInAreaErr error
	syntaxaWithCoverageErr       error

	// syntaxonDistribution and syntaxonCoverage back the syntaxa-distribution
	// read/write pair (subproject C): recorded separately from distribution
	// (species) since the two never share a row.
	syntaxonDistribution []fakeSyntaxonOccurrence
	syntaxonCoverage     []fakeSyntaxonCoverage

	// knownAreaCodes overrides KnownAreaCodes for one scheme, so a test can
	// seed the evc_territory vocabulary without also faking a full
	// species-distribution fixture (which is what the fallback below derives
	// wgsrpd_l3 codes from).
	knownAreaCodes map[string][]string
}

// fakeSyntaxonOccurrence is one recorded UpsertSyntaxonDistribution call.
type fakeSyntaxonOccurrence struct {
	SyntaxonID, Scheme, Code, Occurrence string
}

// fakeSyntaxonCoverage is one recorded UpsertSyntaxonDistributionCoverage call.
type fakeSyntaxonCoverage struct {
	SyntaxonID, Scheme string
}

// fakeDistribution is one recorded UpsertDistribution call.
type fakeDistribution struct {
	ConceptID string
	Area      domain.Area
}

// fakeTraitValue is one recorded UpsertTraitValue call.
type fakeTraitValue struct {
	ConceptID string
	Value     domain.TraitValue
}

// fakeTraitVocab is one recorded UpsertTraitVocabulary call.
type fakeTraitVocab struct {
	Vocab, Version string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{} }

func (r *fakeRepo) ConceptIDs(_ context.Context) ([]string, error) {
	if r.conceptIDsErr != nil {
		return nil, r.conceptIDsErr
	}
	seen := map[string]bool{}
	out := []string{}
	for _, s := range r.speciesRoles {
		if s.ConceptID != nil && !seen[*s.ConceptID] {
			seen[*s.ConceptID] = true
			out = append(out, *s.ConceptID)
		}
	}
	return out, nil
}

func (r *fakeRepo) SearchSpeciesNames(_ context.Context, q string, limit int) ([]domain.SpeciesName, error) {
	if r.searchErr != nil {
		return nil, r.searchErr
	}
	out := []domain.SpeciesName{}
	for _, n := range r.speciesNames {
		if strings.Contains(strings.ToLower(n.VerbatimName), strings.ToLower(q)) {
			out = append(out, n)
		}
	}
	// Mirrors the real adapter's ORDER BY verbatim_name, concept_id: a caller
	// relying on the interface's stable-ordering guarantee must see the same
	// behavior here, including the tie-break when two rows share a name.
	// NULL sorts before any concept id, same as sqlite ASC.
	sort.Slice(out, func(i, j int) bool {
		if out[i].VerbatimName != out[j].VerbatimName {
			return out[i].VerbatimName < out[j].VerbatimName
		}
		a, b := out[i].ConceptID, out[j].ConceptID
		if a == nil {
			return b != nil
		}
		if b == nil {
			return false
		}
		return *a < *b
	})
	// Mirrors SQL's LIMIT semantics, including LIMIT 0 meaning zero rows —
	// the append-then-compare form used before broke exactly on that case.
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

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

// SetSyntaxonParent mirrors the sqlite adapter: an id the index does not
// carry is an error, not a silent no-op, so a test seeding the wrong id
// fails loudly instead of Task 7 checking a no-op that looked like success.
func (r *fakeRepo) SetSyntaxonParent(id, parentID, provenance string) error {
	if err := r.failIfNamed("SetSyntaxonParent"); err != nil {
		return err
	}
	for i := range r.syntaxa {
		if r.syntaxa[i].ID == id {
			r.syntaxa[i].ParentID = parentID
			r.syntaxa[i].ParentProvenance = provenance
			return nil
		}
	}
	return fmt.Errorf("fakeRepo: kein Syntaxon %q", id)
}

// RelinkSyntaxon rewrites every habitat_type_syntaxon edge from from to to,
// dropping the from edge instead of duplicating it if to is already linked
// to the same habitat type.
func (r *fakeRepo) RelinkSyntaxon(from, to string) error {
	if err := r.failIfNamed("RelinkSyntaxon"); err != nil {
		return err
	}
	linked := map[domain.HabitatTypeKey]bool{}
	for _, l := range r.syntaxaLinks {
		if l.syntaxonID == to {
			linked[l.key] = true
		}
	}
	kept := r.syntaxaLinks[:0]
	for _, l := range r.syntaxaLinks {
		switch {
		case l.syntaxonID != from:
			kept = append(kept, l)
		case linked[l.key]:
			// drop: to already carries this edge
		default:
			l.syntaxonID = to
			kept = append(kept, l)
		}
	}
	r.syntaxaLinks = kept
	return nil
}

// SyntaxonIDsByEEACode mirrors the sqlite adapter: rows without an eea code
// are absent from the map.
func (r *fakeRepo) SyntaxonIDsByEEACode(_ context.Context) (map[string]string, error) {
	if r.eeaCodesErr != nil {
		return nil, r.eeaCodesErr
	}
	out := map[string]string{}
	for _, s := range r.syntaxa {
		if s.EEACode != "" {
			out[s.EEACode] = s.ID
		}
	}
	return out, nil
}

// syntaxonByID is a test helper returning the zero value when id is unknown
// — callers assert on the fields they care about, which fail loudly enough.
func (r *fakeRepo) syntaxonByID(id string) domain.Syntaxon {
	for _, s := range r.syntaxa {
		if s.ID == id {
			return s
		}
	}
	return domain.Syntaxon{}
}

// linkTargets is a test helper returning the syntaxon ids linked to
// (typology, code), in insertion order.
func (r *fakeRepo) linkTargets(typology, code string) []string {
	var out []string
	for _, l := range r.syntaxaLinks {
		if string(l.key.Typology) == typology && l.key.Code == code {
			out = append(out, l.syntaxonID)
		}
	}
	return out
}

// area returns the named area matching scheme and code, or the zero value if
// none was written — callers assert on the fields they care about.
func (r *fakeRepo) area(scheme, code string) domain.NamedArea {
	for _, a := range r.areas {
		if a.Scheme == scheme && a.Code == code {
			return a
		}
	}
	return domain.NamedArea{}
}

// hasArea reports whether (scheme, code) was written to the fake index.
func (r *fakeRepo) hasArea(scheme, code string) bool {
	for _, a := range r.areas {
		if a.Scheme == scheme && a.Code == code {
			return true
		}
	}
	return false
}

// has reports whether id was written to the fake index.
func (r *fakeRepo) has(id string) bool {
	for _, s := range r.syntaxa {
		if s.ID == id {
			return true
		}
	}
	return false
}

func (r *fakeRepo) AllSyntaxa(_ context.Context) ([]domain.Syntaxon, error) {
	if r.allSyntaxaErr != nil {
		return nil, r.allSyntaxaErr
	}
	out := make([]domain.Syntaxon, len(r.syntaxa))
	copy(out, r.syntaxa)
	// The interface promises id order (the sqlite adapter does ORDER BY id) —
	// keep the fake honest about it rather than relaxing the contract, even
	// though today's callers don't depend on the order.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
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

func (r *fakeRepo) UpsertDerivedSpeciesRole(role domain.SpeciesRole) (bool, error) {
	if err := r.failIfNamed("UpsertDerivedSpeciesRole"); err != nil {
		return false, err
	}
	for _, existing := range r.speciesRoles {
		if existing.Key == role.Key && existing.VerbatimName == role.VerbatimName && existing.Role == role.Role {
			return true, nil
		}
	}
	r.speciesRoles = append(r.speciesRoles, role)
	return false, nil
}

func (r *fakeRepo) UpsertLocalization(l domain.Localization) error {
	if err := r.failIfNamed("UpsertLocalization"); err != nil {
		return err
	}
	r.localizations = append(r.localizations, l)
	return nil
}

func (r *fakeRepo) UpsertDistribution(conceptID string, a domain.Area) error {
	if err := r.failIfNamed("UpsertDistribution"); err != nil {
		return err
	}
	r.distribution = append(r.distribution, fakeDistribution{ConceptID: conceptID, Area: a})
	return nil
}

func (r *fakeRepo) UpsertArea(a domain.NamedArea) error {
	if err := r.failIfNamed("UpsertArea"); err != nil {
		return err
	}
	r.areas = append(r.areas, a)
	return nil
}

func (r *fakeRepo) UpsertDescription(d domain.HabitatDescription) error {
	if err := r.failIfNamed("UpsertDescription"); err != nil {
		return err
	}
	r.descriptions = append(r.descriptions, d)
	return nil
}

// Description answers from what UpsertDescription recorded, so a query test
// can seed one by writing it.
func (r *fakeRepo) Description(_ context.Context, key domain.HabitatTypeKey) (domain.HabitatDescription, error) {
	if r.descriptionErr != nil {
		return domain.HabitatDescription{}, r.descriptionErr
	}
	for _, d := range r.descriptions {
		if d.Key == key {
			return d, nil
		}
	}
	return domain.HabitatDescription{}, output.ErrNotFound
}

func (r *fakeRepo) UpsertTraitValue(conceptID string, tv domain.TraitValue) error {
	if err := r.failIfNamed("UpsertTraitValue"); err != nil {
		return err
	}
	r.traitValues = append(r.traitValues, fakeTraitValue{ConceptID: conceptID, Value: tv})
	return nil
}

// DeleteTraitValuesForVocab drops every recorded traitValue for vocab,
// mirroring the sqlite adapter's DELETE FROM trait_value WHERE vocab = ?.
func (r *fakeRepo) DeleteTraitValuesForVocab(vocab string) error {
	if err := r.failIfNamed("DeleteTraitValuesForVocab"); err != nil {
		return err
	}
	kept := r.traitValues[:0]
	for _, tv := range r.traitValues {
		if tv.Value.Vocab != vocab {
			kept = append(kept, tv)
		}
	}
	r.traitValues = kept
	return nil
}

func (r *fakeRepo) UpsertTraitVocabulary(vocab, version string) error {
	if err := r.failIfNamed("UpsertTraitVocabulary"); err != nil {
		return err
	}
	r.traitVocabs = append(r.traitVocabs, fakeTraitVocab{Vocab: vocab, Version: version})
	return nil
}

func (r *fakeRepo) Traits(_ context.Context, conceptID string, vocabs []string) ([]domain.TraitSet, error) {
	if r.traitsErr != nil {
		return nil, r.traitsErr
	}
	sets := map[string]*domain.TraitSet{}
	var order []string
	for _, tv := range r.traitValues {
		if tv.ConceptID != conceptID {
			continue
		}
		if len(vocabs) > 0 && !slices.Contains(vocabs, tv.Value.Vocab) {
			continue
		}
		key := tv.Value.Vocab + "@" + tv.Value.VocabVersion
		set, ok := sets[key]
		if !ok {
			set = &domain.TraitSet{Vocab: tv.Value.Vocab, VocabVersion: tv.Value.VocabVersion}
			sets[key] = set
			order = append(order, key)
		}
		set.Values = append(set.Values, tv.Value)
	}
	out := make([]domain.TraitSet, 0, len(order))
	for _, key := range order {
		out = append(out, *sets[key])
	}
	return out, nil
}

// TraitsForConcepts mirrors the sqlite adapter's contract: it is derived
// from the same recorded traitValues as Traits (not a separate store), a
// concept with no rows is absent from the map, and each concept's values
// come out ordered by vocab then dim — the adapter's ORDER BY concept_id,
// vocab, dim.
func (r *fakeRepo) TraitsForConcepts(_ context.Context, conceptIDs []string) (map[string][]domain.TraitValue, error) {
	if r.traitsForConceptsErr != nil {
		return nil, r.traitsForConceptsErr
	}
	wanted := map[string]bool{}
	for _, id := range conceptIDs {
		wanted[id] = true
	}
	out := map[string][]domain.TraitValue{}
	for _, tv := range r.traitValues {
		if wanted[tv.ConceptID] {
			out[tv.ConceptID] = append(out[tv.ConceptID], tv.Value)
		}
	}
	for id := range out {
		values := out[id]
		sort.Slice(values, func(i, j int) bool {
			if values[i].Vocab != values[j].Vocab {
				return values[i].Vocab < values[j].Vocab
			}
			return values[i].Dim < values[j].Dim
		})
	}
	return out, nil
}

func (r *fakeRepo) KnownVocabs(_ context.Context) ([]string, error) {
	if r.knownVocabsErr != nil {
		return nil, r.knownVocabsErr
	}
	seen := map[string]bool{}
	out := []string{}
	for _, tv := range r.traitVocabs {
		if !seen[tv.Vocab] {
			seen[tv.Vocab] = true
			out = append(out, tv.Vocab)
		}
	}
	return out, nil
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

// AreasWithData answers from namedAreas, which a test seeds directly: the
// join of names and distribution rows is the sqlite adapter's job, not this
// double's.
func (r *fakeRepo) AreasWithData(_ context.Context, scheme string) ([]domain.NamedArea, error) {
	if r.areasErr != nil {
		return nil, r.areasErr
	}
	out := []domain.NamedArea{}
	for _, a := range r.namedAreas {
		if a.Scheme == scheme {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *fakeRepo) KnownAreaCodes(_ context.Context, scheme string) ([]string, error) {
	if r.areasErr != nil {
		return nil, r.areasErr
	}
	if codes, ok := r.knownAreaCodes[scheme]; ok {
		return codes, nil
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

// UpsertSyntaxonDistribution mirrors the sqlite adapter's idempotency: a
// repeated call for the same (syntaxonID, scheme, code) overwrites the
// occurrence rather than appending a second row.
func (r *fakeRepo) UpsertSyntaxonDistribution(syntaxonID, scheme, code, occurrence string) error {
	if err := r.failIfNamed("UpsertSyntaxonDistribution"); err != nil {
		return err
	}
	for i, o := range r.syntaxonDistribution {
		if o.SyntaxonID == syntaxonID && o.Scheme == scheme && o.Code == code {
			r.syntaxonDistribution[i].Occurrence = occurrence
			return nil
		}
	}
	r.syntaxonDistribution = append(r.syntaxonDistribution, fakeSyntaxonOccurrence{
		SyntaxonID: syntaxonID, Scheme: scheme, Code: code, Occurrence: occurrence,
	})
	return nil
}

// UpsertSyntaxonDistributionCoverage is idempotent: a second call for the same
// (syntaxonID, scheme) must not duplicate the coverage row.
func (r *fakeRepo) UpsertSyntaxonDistributionCoverage(syntaxonID, scheme string) error {
	if err := r.failIfNamed("UpsertSyntaxonDistributionCoverage"); err != nil {
		return err
	}
	for _, c := range r.syntaxonCoverage {
		if c.SyntaxonID == syntaxonID && c.Scheme == scheme {
			return nil
		}
	}
	r.syntaxonCoverage = append(r.syntaxonCoverage, fakeSyntaxonCoverage{SyntaxonID: syntaxonID, Scheme: scheme})
	return nil
}

// SyntaxonDistribution mirrors the sqlite adapter's contract: Covered false
// with both lists empty when no coverage row was ever written, distinct from
// Covered true with empty lists (the source stated coverage but no occurrence).
func (r *fakeRepo) SyntaxonDistribution(_ context.Context, syntaxonID, scheme string) (domain.SyntaxonDistribution, error) {
	if r.syntaxonDistributionErr != nil {
		return domain.SyntaxonDistribution{}, r.syntaxonDistributionErr
	}
	out := domain.SyntaxonDistribution{Scheme: scheme}
	for _, c := range r.syntaxonCoverage {
		if c.SyntaxonID == syntaxonID && c.Scheme == scheme {
			out.Covered = true
			break
		}
	}
	for _, o := range r.syntaxonDistribution {
		if o.SyntaxonID != syntaxonID || o.Scheme != scheme {
			continue
		}
		switch o.Occurrence {
		case domain.OccurrenceVerified:
			out.Verified = append(out.Verified, o.Code)
		case domain.OccurrenceUncertain:
			out.Uncertain = append(out.Uncertain, o.Code)
		}
	}
	sort.Strings(out.Verified)
	sort.Strings(out.Uncertain)
	return out, nil
}

// SyntaxonOccurrencesInArea maps syntaxon id -> occurrence for one area,
// mirroring the sqlite adapter.
func (r *fakeRepo) SyntaxonOccurrencesInArea(_ context.Context, scheme, code string) (map[string]string, error) {
	if r.syntaxonOccurrencesInAreaErr != nil {
		return nil, r.syntaxonOccurrencesInAreaErr
	}
	out := map[string]string{}
	for _, o := range r.syntaxonDistribution {
		if o.Scheme == scheme && o.Code == code {
			out[o.SyntaxonID] = o.Occurrence
		}
	}
	return out, nil
}

// SyntaxaWithCoverage returns the set of syntaxa the source makes a statement
// about, for one scheme.
func (r *fakeRepo) SyntaxaWithCoverage(_ context.Context, scheme string) (map[string]bool, error) {
	if r.syntaxaWithCoverageErr != nil {
		return nil, r.syntaxaWithCoverageErr
	}
	out := map[string]bool{}
	for _, c := range r.syntaxonCoverage {
		if c.Scheme == scheme {
			out[c.SyntaxonID] = true
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

func (r *fakeRepo) Localization(_ context.Context, entityType, entityKey, lang string) ([]domain.Localization, error) {
	r.localizationCalls++
	if r.localizationErr != nil {
		return nil, r.localizationErr
	}
	if r.localizationErrOnCall != 0 && r.localizationCalls == r.localizationErrOnCall {
		return nil, fmt.Errorf("fakeRepo: injected Localization failure on call %d", r.localizationCalls)
	}
	var out []domain.Localization
	for _, l := range r.localizations {
		if l.EntityType == entityType && l.EntityKey == entityKey && l.Lang == lang {
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
