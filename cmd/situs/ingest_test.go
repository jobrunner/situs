package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubHostus points SITUS_HOSTUS_BASE_URL at a server that resolves nothing —
// these tests exercise the ingest wiring, not the hostus crosswalk itself.
func stubHostus(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SITUS_HOSTUS_BASE_URL", srv.URL)
}

// captureStdout redirects os.Stdout for the duration of fn and returns what
// was written to it. The ingest command's logger writes there directly
// (mirroring serve's setupLogger(cfg.Logging, os.Stdout)), separate from
// cmd.OutOrStdout(), which carries the JSON report.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return string(data)
}

func writeIngestCSV(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func seedIngestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeIngestCSV(t, dir, "typologies.csv",
		"id,scheme,version,name,source_ref\neunis@2021,eunis,2021,EUNIS 2021,https://example.org\n")
	writeIngestCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\neunis@2021,R22,3,Hay meadow,R2,\n")
	writeIngestCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n")
	writeIngestCSV(t, dir, "syntaxa.csv", "id,rank,name,parent_id\n")
	writeIngestCSV(t, dir, "habitat_type_syntaxa.csv", "typology_id,code,syntaxon_id\n")
	writeIngestCSV(t, dir, "species_roles.csv",
		"typology_id,code,verbatim_name,role,fidelity,constancy\neunis@2021,R22,Inula hirta,diagnostic,0.8,\n")
	return dir
}

func TestIngestCommandLoadsCSVsAndPrintsTheReport(t *testing.T) {
	stubHostus(t)
	csvDir := seedIngestDir(t)
	dbPath := filepath.Join(t.TempDir(), "situs.sqlite")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"ingest", "--csv-dir", csvDir, "--db", dbPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("executing ingest: %v", err)
	}
	if !strings.Contains(out.String(), `"HabitatTypes": 1`) {
		t.Errorf("output = %q, want it to report one habitat type", out.String())
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("sqlite index was not created: %v", err)
	}
}

func TestIngestCommandFailsOnAMissingCSVDirectory(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"ingest", "--csv-dir", filepath.Join(t.TempDir(), "missing"), "--db", filepath.Join(t.TempDir(), "situs.sqlite")})

	if err := root.Execute(); err == nil {
		t.Fatal("executing ingest with a missing csv-dir = nil error, want an error")
	}
}

func TestIngestCommandFailsOnAnUnreachableDB(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"ingest", "--csv-dir", seedIngestDir(t), "--db", t.TempDir()})

	if err := root.Execute(); err == nil {
		t.Fatal("executing ingest with an unreachable db path = nil error, want an error")
	}
}

func TestIngestCommandRequiresTheCSVDirectory(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"ingest"})

	err := root.Execute()
	if err == nil {
		t.Fatal("executing ingest without flags = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "--csv-dir") {
		t.Errorf("error = %q, want it to name the required flag", err)
	}
}

// Ingesting into one file while serving another yields an empty-but-healthy
// service, so --db falls back to the same index.path that serve reads.
func TestIngestCommandDefaultsTheDBToTheConfiguredIndexPath(t *testing.T) {
	stubHostus(t)
	dbPath := filepath.Join(t.TempDir(), "from-config.sqlite")
	t.Setenv("SITUS_INDEX_PATH", dbPath)

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"ingest", "--csv-dir", seedIngestDir(t)})

	captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Errorf("executing ingest without --db = %v, want it to use index.path", err)
		}
	})
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("stat %s = %v, want ingest to have written the configured index", dbPath, err)
	}
}

// The flag overrides config, as everywhere else in this codebase.
func TestIngestCommandDBFlagOverridesTheConfiguredIndexPath(t *testing.T) {
	stubHostus(t)
	dir := t.TempDir()
	fromConfig := filepath.Join(dir, "from-config.sqlite")
	fromFlag := filepath.Join(dir, "from-flag.sqlite")
	t.Setenv("SITUS_INDEX_PATH", fromConfig)

	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"ingest", "--csv-dir", seedIngestDir(t), "--db", fromFlag})

	captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Errorf("executing ingest = %v, want no error", err)
		}
	})
	if _, err := os.Stat(fromFlag); err != nil {
		t.Errorf("stat %s = %v, want the flag to have won", fromFlag, err)
	}
	if _, err := os.Stat(fromConfig); err == nil {
		t.Errorf("%s exists, want the flag to override index.path entirely", fromConfig)
	}
}

// The only record of a dropped row is the log stream; it must honor the
// service's own logging config (SITUS_LOGGING_FORMAT), not slog's
// unconfigured default text handler.
func TestIngestCommandRoutesSkipWarningsThroughTheConfiguredLogger(t *testing.T) {
	stubHostus(t)
	dir := seedIngestDir(t)
	writeIngestCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\neunis@2021,R22,annex1,6510,~\n")

	t.Setenv("SITUS_LOGGING_FORMAT", "json")
	t.Setenv("SITUS_LOGGING_LEVEL", "warn")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"ingest", "--csv-dir", dir, "--db", filepath.Join(t.TempDir(), "situs.sqlite")})

	logOutput := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("executing ingest: %v", err)
		}
	})

	if !strings.Contains(out.String(), `"SkippedRows": 1`) {
		t.Errorf("report = %q, want it to report the skipped row", out.String())
	}
	if !strings.Contains(logOutput, `"file":"crosswalks.csv"`) {
		t.Errorf("log output = %q, want configured JSON logging naming the file", logOutput)
	}
	if !strings.Contains(logOutput, `"line":2`) {
		t.Errorf("log output = %q, want configured JSON logging naming the line", logOutput)
	}
}
