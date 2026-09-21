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

// writeAreaCSV writes csv into a temp dir and returns its path.
func writeAreaCSV(t *testing.T, csv string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wgsrpd_areas.csv")
	if err := os.WriteFile(path, []byte(csv), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

const areaHeader = "area_scheme,area_code,name_en\n"

func TestIngestAreas_WritesEveryRow(t *testing.T) {
	path := writeAreaCSV(t, areaHeader+
		"wgsrpd_l3,GER,Germany\n"+
		"wgsrpd_l3,FOR,Føroyar\n")
	repo := newFakeRepo()

	rep, err := IngestAreas(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestAreas: %v", err)
	}
	if rep.Areas != 2 {
		t.Errorf("Areas = %d, want 2", rep.Areas)
	}
	if !repo.committed {
		t.Error("ingest did not commit")
	}
	want := []domain.NamedArea{
		{Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}, NameEN: "Germany"},
		{Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "FOR"}, NameEN: "Føroyar"},
	}
	for i, w := range want {
		if i >= len(repo.areas) || repo.areas[i] != w {
			t.Fatalf("areas = %+v, want %+v (encoding must survive the pipeline)", repo.areas, want)
		}
	}
}

// An incomplete area would be a row that can never match anything — it is
// counted as skipped, never written.
func TestIngestAreas_SkipsIncompleteRows(t *testing.T) {
	path := writeAreaCSV(t, areaHeader+
		"wgsrpd_l3,GER,Germany\n"+
		",FRA,France\n"+
		"wgsrpd_l3,,Nowhere\n")
	repo := newFakeRepo()

	rep, err := IngestAreas(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestAreas: %v", err)
	}
	if rep.Areas != 1 || rep.SkippedRows != 2 {
		t.Errorf("report = %+v, want 1 area and 2 skipped rows", rep)
	}
}

// A name is allowed to be empty: the code is the fact, the name the overlay.
func TestIngestAreas_KeepsARowWithoutAName(t *testing.T) {
	path := writeAreaCSV(t, areaHeader+"wgsrpd_l3,GER,\n")
	repo := newFakeRepo()

	rep, err := IngestAreas(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestAreas: %v", err)
	}
	if rep.Areas != 1 || len(repo.areas) != 1 || repo.areas[0].NameEN != "" {
		t.Errorf("report = %+v, areas = %+v, want the unnamed code written", rep, repo.areas)
	}
}

// No pinned WGSRPD artifact yet is a normal state, same as the syntaxa
// hierarchy: the ingest keeps running, the codes simply stay unnamed.
func TestIngestAreas_MissingFileIsNotAnError(t *testing.T) {
	repo := newFakeRepo()

	rep, err := IngestAreas(context.Background(), repo, filepath.Join(t.TempDir(), "absent.csv"))
	if err != nil {
		t.Fatalf("IngestAreas on a missing file: %v", err)
	}
	if rep.Areas != 0 {
		t.Errorf("report = %+v, want an empty report", rep)
	}
	if repo.committed {
		t.Error("a skipped ingest must not open and commit a transaction")
	}
}

func TestIngestAreas_HeaderMismatchIsAnError(t *testing.T) {
	path := writeAreaCSV(t, "scheme,code,name\nwgsrpd_l3,GER,Germany\n")

	if _, err := IngestAreas(context.Background(), newFakeRepo(), path); err == nil {
		t.Fatal("IngestAreas with a wrong header = nil error, want an error")
	}
}

func TestIngestAreas_RepositoryErrorRollsBack(t *testing.T) {
	path := writeAreaCSV(t, areaHeader+"wgsrpd_l3,GER,Germany\n")
	repo := newFakeRepo()
	repo.failOn = "UpsertArea"

	if _, err := IngestAreas(context.Background(), repo, path); err == nil {
		t.Fatal("IngestAreas with UpsertArea failing = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite UpsertArea failing")
	}
	if !repo.rolledBack {
		t.Error("ingest did not roll back after UpsertArea failed")
	}
}

func TestIngestAreas_BeginErrorIsReturned(t *testing.T) {
	path := writeAreaCSV(t, areaHeader+"wgsrpd_l3,GER,Germany\n")
	repo := newFakeRepo()
	repo.beginErr = errors.New("index locked")

	if _, err := IngestAreas(context.Background(), repo, path); err == nil {
		t.Fatal("IngestAreas with Begin failing = nil error, want an error")
	}
}

func TestIngestAreas_CommitErrorIsReturned(t *testing.T) {
	path := writeAreaCSV(t, areaHeader+"wgsrpd_l3,GER,Germany\n")
	repo := newFakeRepo()
	repo.commitErr = errors.New("disk full")

	if _, err := IngestAreas(context.Background(), repo, path); err == nil {
		t.Fatal("IngestAreas with Commit failing = nil error, want an error")
	}
}

// An unreadable path is not "no file pinned": only os.IsNotExist may be
// skipped, everything else has to surface.
func TestIngestAreas_UnreadablePathIsAnError(t *testing.T) {
	notADir := writeAreaCSV(t, areaHeader)
	// A path *below* a regular file: ENOTDIR, which is not os.IsNotExist.
	path := filepath.Join(notADir, "wgsrpd_areas.csv")

	if _, err := IngestAreas(context.Background(), newFakeRepo(), path); err == nil {
		t.Fatal("IngestAreas on an unreadable path = nil error, want an error")
	}
}

// A failed rollback must not hide the error that caused it.
func TestIngestAreas_FailedRollbackIsReportedAlongsideTheCause(t *testing.T) {
	path := writeAreaCSV(t, areaHeader+"wgsrpd_l3,GER,Germany\n")
	repo := newFakeRepo()
	repo.failOn = "UpsertArea"
	repo.rollbackErr = errors.New("connection lost")

	_, err := IngestAreas(context.Background(), repo, path)
	if err == nil {
		t.Fatal("IngestAreas = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "connection lost") || !strings.Contains(err.Error(), "UpsertArea") {
		t.Errorf("error = %q, want both the rollback failure and its cause", err)
	}
}

// The skipped-row warning is the only record a dropped row leaves. It has to
// fire when something was dropped — and stay silent when nothing was, so a
// clean run does not look like a lossy one.
func TestIngestAreas_WarnsOnlyWhenRowsWereSkipped(t *testing.T) {
	cases := map[string]struct {
		csv      string
		wantWarn bool
	}{
		"one skipped row": {csv: areaHeader + "wgsrpd_l3,GER,Germany\n,FRA,France\n", wantWarn: true},
		"nothing skipped": {csv: areaHeader + "wgsrpd_l3,GER,Germany\n", wantWarn: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			log := captureSlog(t)

			if _, err := IngestAreas(context.Background(), newFakeRepo(), writeAreaCSV(t, tc.csv)); err != nil {
				t.Fatalf("IngestAreas: %v", err)
			}
			warned := strings.Contains(log.String(), "skipped malformed rows in the area name file")
			if warned != tc.wantWarn {
				t.Errorf("warning logged = %v, want %v; log: %s", warned, tc.wantWarn, log)
			}
		})
	}
}

// situs stores exactly one area scheme. A row naming another one would be
// written and counted as a success, yet join with nothing on the read side —
// a silent data defect wearing a clean report. It is skipped and counted
// instead, so a typo in the pipeline surfaces where it is measured.
func TestIngestAreas_SkipsAForeignAreaScheme(t *testing.T) {
	path := writeAreaCSV(t, areaHeader+
		"wgsrpd_l3,GER,Germany\n"+
		"wgsrpd_13,AUT,Austria\n") // digit one-three, not letter-l-three
	repo := newFakeRepo()

	rep, err := IngestAreas(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestAreas: %v", err)
	}
	if rep.Areas != 1 || rep.SkippedRows != 1 {
		t.Errorf("report = %+v, want 1 area and 1 skipped row", rep)
	}
	if len(repo.areas) != 1 || repo.areas[0].Code != "GER" {
		t.Errorf("areas = %+v, want only the row in the one scheme situs stores", repo.areas)
	}
}

func TestIngestAreasSchreibtBeideSchemata(t *testing.T) {
	for _, tc := range []struct{ name, scheme, code, nameEN string }{
		{"wgsrpd", "wgsrpd_l3", "GER", "Germany"},
		{"evc", "evc_territory", "austria-alps", "Austria Alps"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			path := filepath.Join(t.TempDir(), "areas.csv")
			content := "area_scheme,area_code,name_en\n" + tc.scheme + "," + tc.code + "," + tc.nameEN + "\n"
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("Schreiben: %v", err)
			}

			rep, err := IngestAreas(context.Background(), repo, path)
			if err != nil {
				t.Fatalf("IngestAreas: %v", err)
			}
			if rep.Areas != 1 || rep.SkippedRows != 0 {
				t.Fatalf("Report = %+v, erwartet 1 Gebiet und 0 uebersprungene", rep)
			}
			got := repo.area(tc.scheme, tc.code)
			if got.NameEN != tc.nameEN {
				t.Errorf("Name = %q, erwartet %q", got.NameEN, tc.nameEN)
			}
		})
	}
}

func TestIngestAreasUeberspringtUnbekanntesSchema(t *testing.T) {
	// A typo must still be caught. "evc-territory" with a hyphen is the exact
	// mistake the underscore decision in the spec exists to keep visible.
	repo := newFakeRepo()
	path := filepath.Join(t.TempDir(), "areas.csv")
	content := "area_scheme,area_code,name_en\n" +
		"evc-territory,austria-alps,Austria Alps\n" +
		"iso3166,DE,Germany\n" +
		"evc_territory,albania,Albania\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("Schreiben: %v", err)
	}

	rep, err := IngestAreas(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestAreas: %v", err)
	}
	if rep.Areas != 1 || rep.SkippedRows != 2 {
		t.Errorf("Report = %+v, erwartet 1 Gebiet und 2 uebersprungene", rep)
	}
	if repo.hasArea("evc-territory", "austria-alps") {
		t.Error("das Bindestrich-Schema wurde geschrieben")
	}
}

// `situs ingest --csv-dir .` hands IngestAreas a bare relative filename, whose
// directory half is "" — which os.OpenRoot (csvReader's confinement) does not
// read as "the current directory". The file exists, so the ingest must read it.
func TestIngestAreas_ReadsABareRelativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wgsrpd_areas.csv"),
		[]byte(areaHeader+"wgsrpd_l3,GER,Germany\n"), 0o600); err != nil {
		t.Fatalf("writing the CSV: %v", err)
	}
	t.Chdir(dir)
	repo := newFakeRepo()

	rep, err := IngestAreas(context.Background(), repo, "wgsrpd_areas.csv")
	if err != nil {
		t.Fatalf("IngestAreas on a bare relative path: %v", err)
	}
	if rep.Areas != 1 {
		t.Errorf("report = %+v, want the one row read", rep)
	}
}
