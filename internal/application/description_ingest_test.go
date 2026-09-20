package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

const descHeader = "typology_id,code,description_en\n"

func writeDescCSV(t *testing.T, csv string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "habitat_descriptions.csv")
	if err := os.WriteFile(path, []byte(csv), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// repoWithTypes seeds the habitat types a description may attach to.
func repoWithTypes(codes ...string) *fakeRepo {
	r := newFakeRepo()
	for _, c := range codes {
		r.types = append(r.types, domain.HabitatType{
			Key: domain.HabitatTypeKey{Typology: "eunis@2021", Code: c},
		})
	}
	return r
}

func TestIngestDescriptions_WritesEveryRow(t *testing.T) {
	path := writeDescCSV(t, descHeader+
		"eunis@2021,R22,\"Hay meadows of lowland areas.\"\n"+
		"eunis@2021,N11,\"Sand beaches.\"\n")
	repo := repoWithTypes("R22", "N11")

	rep, err := IngestDescriptions(context.Background(), repo, path, "floraveg:2021-06-01", domain.DescriptionProvenanceOfficial)
	if err != nil {
		t.Fatalf("IngestDescriptions: %v", err)
	}
	if rep.Written != 2 || rep.SkippedUnknownCode != 0 {
		t.Errorf("report = %+v, want 2 written", rep)
	}
	if len(repo.descriptions) != 2 || repo.descriptions[0].TextEN != "Hay meadows of lowland areas." {
		t.Fatalf("descriptions = %+v", repo.descriptions)
	}
	if repo.descriptions[0].Source != "floraveg:2021-06-01" {
		t.Errorf("source = %q, want the artifact the text came from", repo.descriptions[0].Source)
	}
	if repo.descriptions[0].Provenance != domain.DescriptionProvenanceOfficial {
		t.Errorf("provenance = %q, want it set from the call", repo.descriptions[0].Provenance)
	}
}

// The factsheets cover habitats this index does not carry (11 marine MA*
// codes, N23, N24 — measured). A description pointing at no habitat type is
// dropped and counted, never stored where nothing can reach it.
func TestIngestDescriptions_DropsAndCountsUnknownCodes(t *testing.T) {
	path := writeDescCSV(t, descHeader+
		"eunis@2021,R22,\"Hay meadows.\"\n"+
		"eunis@2021,MA211,\"Arctic salt marshes.\"\n")
	repo := repoWithTypes("R22")

	rep, err := IngestDescriptions(context.Background(), repo, path, "src", domain.DescriptionProvenanceOfficial)
	if err != nil {
		t.Fatalf("IngestDescriptions: %v", err)
	}
	if rep.Written != 1 || rep.SkippedUnknownCode != 1 {
		t.Errorf("report = %+v, want 1 written and 1 dropped", rep)
	}
	if len(repo.descriptions) != 1 || repo.descriptions[0].Key.Code != "R22" {
		t.Errorf("descriptions = %+v, want only the known code", repo.descriptions)
	}
}

func TestIngestDescriptions_SkipsIncompleteRows(t *testing.T) {
	path := writeDescCSV(t, descHeader+
		"eunis@2021,R22,\"Hay meadows.\"\n"+
		"eunis@2021,N11,\n"+
		"eunis@2021,,\"No code at all.\"\n")
	repo := repoWithTypes("R22", "N11")

	rep, err := IngestDescriptions(context.Background(), repo, path, "src", domain.DescriptionProvenanceOfficial)
	if err != nil {
		t.Fatalf("IngestDescriptions: %v", err)
	}
	if rep.Written != 1 || rep.SkippedRows != 2 {
		t.Errorf("report = %+v, want 1 written and 2 skipped rows", rep)
	}
}

// A typology id that cannot be parsed is a malformed row, not an unknown
// habitat type — the two counters answer different questions.
func TestIngestDescriptions_MalformedTypologyIsASkippedRow(t *testing.T) {
	path := writeDescCSV(t, descHeader+"eunis@,R22,\"Hay meadows.\"\n")
	repo := repoWithTypes("R22")

	rep, err := IngestDescriptions(context.Background(), repo, path, "src", domain.DescriptionProvenanceOfficial)
	if err != nil {
		t.Fatalf("IngestDescriptions: %v", err)
	}
	if rep.Written != 0 || rep.SkippedRows != 1 || rep.SkippedUnknownCode != 0 {
		t.Errorf("report = %+v, want the malformed typology counted as a skipped row", rep)
	}
}

func TestIngestDescriptions_MissingFileIsNotAnError(t *testing.T) {
	repo := newFakeRepo()

	rep, err := IngestDescriptions(context.Background(), repo, filepath.Join(t.TempDir(), "absent.csv"), "src", domain.DescriptionProvenanceOfficial)
	if err != nil {
		t.Fatalf("IngestDescriptions on a missing file: %v", err)
	}
	if rep.Written != 0 || repo.committed {
		t.Errorf("report = %+v, committed = %v, want an untouched index", rep, repo.committed)
	}
}

func TestIngestDescriptions_HeaderMismatchIsAnError(t *testing.T) {
	path := writeDescCSV(t, "typology,code,text\neunis@2021,R22,x\n")

	if _, err := IngestDescriptions(context.Background(), newFakeRepo(), path, "src", domain.DescriptionProvenanceOfficial); err == nil {
		t.Fatal("IngestDescriptions with a wrong header = nil error, want an error")
	}
}

func TestIngestDescriptions_LookupErrorIsReturned(t *testing.T) {
	path := writeDescCSV(t, descHeader+"eunis@2021,R22,\"Hay meadows.\"\n")
	repo := repoWithTypes("R22")
	repo.habitatTypeErr = errors.New("index unreadable")

	if _, err := IngestDescriptions(context.Background(), repo, path, "src", domain.DescriptionProvenanceOfficial); err == nil {
		t.Fatal("IngestDescriptions with the lookup failing = nil error, want an error")
	}
}

func TestIngestDescriptions_RepositoryErrorRollsBack(t *testing.T) {
	path := writeDescCSV(t, descHeader+"eunis@2021,R22,\"Hay meadows.\"\n")
	repo := repoWithTypes("R22")
	repo.failOn = "UpsertDescription"

	if _, err := IngestDescriptions(context.Background(), repo, path, "src", domain.DescriptionProvenanceOfficial); err == nil {
		t.Fatal("IngestDescriptions with UpsertDescription failing = nil error, want an error")
	}
	if repo.committed || !repo.rolledBack {
		t.Errorf("committed = %v, rolledBack = %v, want a rollback", repo.committed, repo.rolledBack)
	}
}

func TestIngestDescriptions_BeginAndCommitErrorsAreReturned(t *testing.T) {
	path := writeDescCSV(t, descHeader+"eunis@2021,R22,\"Hay meadows.\"\n")
	for name, prepare := range map[string]func(*fakeRepo){
		"Begin":  func(r *fakeRepo) { r.beginErr = errors.New("index locked") },
		"Commit": func(r *fakeRepo) { r.commitErr = errors.New("disk full") },
	} {
		t.Run(name, func(t *testing.T) {
			repo := repoWithTypes("R22")
			prepare(repo)
			if _, err := IngestDescriptions(context.Background(), repo, path, "src", domain.DescriptionProvenanceOfficial); err == nil {
				t.Fatalf("IngestDescriptions with %s failing = nil error, want an error", name)
			}
		})
	}
}

func TestIngestDescriptions_FailedRollbackIsReportedAlongsideTheCause(t *testing.T) {
	path := writeDescCSV(t, descHeader+"eunis@2021,R22,\"Hay meadows.\"\n")
	repo := repoWithTypes("R22")
	repo.failOn = "UpsertDescription"
	repo.rollbackErr = errors.New("connection lost")

	_, err := IngestDescriptions(context.Background(), repo, path, "src", domain.DescriptionProvenanceOfficial)
	if err == nil {
		t.Fatal("IngestDescriptions = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "connection lost") || !strings.Contains(err.Error(), "UpsertDescription") {
		t.Errorf("error = %q, want both the rollback failure and its cause", err)
	}
}

func TestIngestDescriptions_UnreadablePathIsAnError(t *testing.T) {
	notADir := writeDescCSV(t, descHeader)

	if _, err := IngestDescriptions(context.Background(), newFakeRepo(),
		filepath.Join(notADir, "habitat_descriptions.csv"), "src", domain.DescriptionProvenanceOfficial); err == nil {
		t.Fatal("IngestDescriptions on an unreadable path = nil error, want an error")
	}
}

func TestIngestDescriptions_WarnsOnlyWhenRowsWereSkipped(t *testing.T) {
	cases := map[string]struct {
		csv      string
		wantWarn bool
	}{
		"one skipped": {csv: descHeader + "eunis@2021,R22,\"Meadows.\"\neunis@2021,,\"x\"\n", wantWarn: true},
		"none":        {csv: descHeader + "eunis@2021,R22,\"Meadows.\"\n", wantWarn: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			log := captureSlog(t)
			if _, err := IngestDescriptions(context.Background(), repoWithTypes("R22"), writeDescCSV(t, tc.csv), "src", domain.DescriptionProvenanceOfficial); err != nil {
				t.Fatalf("IngestDescriptions: %v", err)
			}
			warned := strings.Contains(log.String(), "skipped malformed rows in the description file")
			if warned != tc.wantWarn {
				t.Errorf("warning logged = %v, want %v; log: %s", warned, tc.wantWarn, log)
			}
		})
	}
}

// The dropped-descriptions line is the only record that the factsheets cover
// habitats this index does not. It must fire when something was dropped and
// stay silent when nothing was.
func TestIngestDescriptions_LogsDroppedCodesOnlyWhenThereWereAny(t *testing.T) {
	cases := map[string]struct {
		csv     string
		wantLog bool
	}{
		"one dropped": {csv: descHeader + "eunis@2021,R22,\"Meadows.\"\neunis@2021,MA211,\"Salt marsh.\"\n", wantLog: true},
		"none":        {csv: descHeader + "eunis@2021,R22,\"Meadows.\"\n", wantLog: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			log := captureSlog(t)
			if _, err := IngestDescriptions(context.Background(), repoWithTypes("R22"), writeDescCSV(t, tc.csv), "src", domain.DescriptionProvenanceOfficial); err != nil {
				t.Fatalf("IngestDescriptions: %v", err)
			}
			logged := strings.Contains(log.String(), "dropped descriptions for habitat types this index does not carry")
			if logged != tc.wantLog {
				t.Errorf("line logged = %v, want %v; log: %s", logged, tc.wantLog, log)
			}
		})
	}
}

// Two sources feed the same table: the EUNIS factsheets (official wording)
// and the Annex I texts situs writes itself. The ingest stamps which is which
// per file; nothing about the row's content decides it.
func TestIngestDescriptions_StampsTheProvenanceOfTheFile(t *testing.T) {
	path := writeDescCSV(t, descHeader+"eunis@2021,R22,\"Hay meadows.\"\n")
	repo := repoWithTypes("R22")

	if _, err := IngestDescriptions(context.Background(), repo, path, "situs@test",
		domain.DescriptionProvenanceSitus); err != nil {
		t.Fatalf("IngestDescriptions: %v", err)
	}
	if repo.descriptions[0].Provenance != domain.DescriptionProvenanceSitus {
		t.Errorf("provenance = %q, want situs", repo.descriptions[0].Provenance)
	}
}
