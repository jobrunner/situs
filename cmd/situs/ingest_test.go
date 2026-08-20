package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jobrunner/situs/internal/domain"
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

// fakeSequentialSource records the concept ids each Areas call carries, so a
// test can pin that pacedDistributionSource asks one id at a time. failOn
// names the one concept id whose single-concept request should fail; every
// other id succeeds.
type fakeSequentialSource struct {
	asked  [][]string
	err    error
	failOn string
}

func (f *fakeSequentialSource) Areas(_ context.Context, ids []string) (map[string][]domain.Area, error) {
	f.asked = append(f.asked, ids)
	if f.err != nil {
		return nil, f.err
	}
	out := map[string][]domain.Area{}
	for _, id := range ids {
		if id == f.failOn {
			return nil, fmt.Errorf("hostus: %s unavailable", id)
		}
		out[id] = []domain.Area{{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}}
	}
	return out, nil
}

func TestPacedDistributionSource_AsksOneConceptAtATime(t *testing.T) {
	src := &fakeSequentialSource{}
	paced := &pacedDistributionSource{src: src, pause: time.Millisecond}

	got, err := paced.Areas(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %v, want areas for all three concepts", got)
	}
	if len(src.asked) != 3 {
		t.Errorf("source was asked %d times, want once per concept", len(src.asked))
	}
	for _, ids := range src.asked {
		if len(ids) != 1 {
			t.Errorf("asked for %v, want exactly one id per call", ids)
		}
	}
}

// A timeout on one concept out of many must not throw away the rest of an
// otherwise-successful batch.
func TestPacedDistributionSource_ToleratesASingleConceptFailureAndKeepsTheRest(t *testing.T) {
	src := &fakeSequentialSource{failOn: "b"}
	paced := &pacedDistributionSource{src: src, pause: time.Millisecond}

	got, err := paced.Areas(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if _, ok := got["a"]; !ok {
		t.Error("concept a is missing, want it kept despite b's failure")
	}
	if _, ok := got["c"]; !ok {
		t.Error("concept c is missing, want it kept despite b's failure")
	}
	if _, ok := got["b"]; ok {
		t.Error("concept b should have no areas, its request failed")
	}
	if got := paced.FailedConcepts(); got != 1 {
		t.Errorf("FailedConcepts() = %d, want 1", got)
	}
}

// A real outage must not put one line per failed concept in the log — only
// the first few individual failures get their own line, and the aggregate
// at the end always says how many there really were.
func TestPacedDistributionSource_CapsPerConceptFailureLogLines(t *testing.T) {
	src := &fakeSequentialSource{}
	paced := &pacedDistributionSource{src: src, pause: time.Millisecond}
	ids := []string{"a", "b", "c", "d", "e"}
	// Every id fails except the last, so this is a partial (not whole-batch)
	// failure with more failures than maxLoggedConceptFailures.
	failing := &fakeConditionalSource{failExcept: "e"}
	paced.src = failing

	logOutput := captureLogOutput(t, func() {
		if _, err := paced.Areas(context.Background(), ids); err != nil {
			t.Fatalf("Areas: %v", err)
		}
	})

	individualLines := strings.Count(logOutput, "distribution request for one concept failed")
	if individualLines != maxLoggedConceptFailures {
		t.Errorf("logged %d individual failure lines, want exactly %d (the cap)", individualLines, maxLoggedConceptFailures)
	}
	if !strings.Contains(logOutput, "some distribution requests failed") {
		t.Error("log output is missing the aggregate line")
	}
	if !strings.Contains(logOutput, "failed=4") {
		t.Errorf("log output = %q, want the aggregate line to name the true failure count (4), not just the capped log lines", logOutput)
	}
}

// fakeConditionalSource fails every concept id except failExcept.
type fakeConditionalSource struct {
	failExcept string
}

func (f *fakeConditionalSource) Areas(_ context.Context, ids []string) (map[string][]domain.Area, error) {
	id := ids[0]
	if id != f.failExcept {
		return nil, fmt.Errorf("hostus: %s unavailable", id)
	}
	return map[string][]domain.Area{id: {{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}}}, nil
}

// captureLogOutput points the default slog logger at a buffer for fn's
// duration. pacedDistributionSource logs via slog.WarnContext against
// whatever the default logger is; the ingest command itself installs its own
// via installLogger, but these decorator-level tests call Areas directly.
func captureLogOutput(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)
	fn()
	return buf.String()
}

// If every single request fails, Areas must report that as one whole-batch
// failure — not as a "successful" empty map — so IngestDistribution takes
// the same all-or-nothing warn-and-zero path as a plain source outage.
func TestPacedDistributionSource_AllConceptsFailingIsAWholeBatchFailure(t *testing.T) {
	paced := &pacedDistributionSource{src: &fakeSequentialSource{err: errors.New("upstream down")}, pause: time.Millisecond}

	got, err := paced.Areas(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatal("Areas: want an error when every request failed, got nil")
	}
	if got != nil {
		t.Errorf("Areas returned %v, want a nil map on whole-batch failure", got)
	}
	if failed := paced.FailedConcepts(); failed != 0 {
		t.Errorf("FailedConcepts() = %d, want 0 — a whole-batch failure is folded into the ordinary outage path, not a partial-failure count", failed)
	}
}

func TestPacedDistributionSource_StopsWhenTheContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	paced := &pacedDistributionSource{src: &fakeSequentialSource{}, pause: time.Hour}

	_, err := paced.Areas(ctx, []string{"a", "b"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Areas returned %v, want context.Canceled", err)
	}
}

// A canceled context surfacing from the underlying source (mid-request, not
// between requests) must abort the whole batch too, not be folded into the
// per-concept tolerance.
func TestPacedDistributionSource_PropagatesContextCancellationFromTheSource(t *testing.T) {
	paced := &pacedDistributionSource{src: &fakeSequentialSource{err: context.Canceled}, pause: time.Millisecond}

	_, err := paced.Areas(context.Background(), []string{"a", "b"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Areas returned %v, want context.Canceled to propagate", err)
	}
}

// TestIngestCommand_ReportsFailedConceptsFromThePacedDecorator drives the
// full ingest command against a stub hostus that resolves two species to two
// concepts and then fails one concept's GET /v1/concept/{id} — pinning that
// runIngest reads pacedDistributionSource.FailedConcepts() into the printed
// report as DistributionFailed.
func TestIngestCommand_ReportsFailedConceptsFromThePacedDecorator(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/match":
			_, _ = w.Write([]byte(`{"results":[
				{"id":"0","concept_id":"wcvp:concept:1","match_type":"exact"},
				{"id":"1","concept_id":"wcvp:concept:2","match_type":"exact"}
			]}`))
		case strings.HasSuffix(r.URL.Path, "wcvp:concept:1"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		case strings.HasSuffix(r.URL.Path, "wcvp:concept:2"):
			_, _ = w.Write([]byte(`{"distribution":[{"area_scheme":"wgsrpd_l3","area_code":"GER"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SITUS_HOSTUS_BASE_URL", srv.URL)

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
		"typology_id,code,verbatim_name,role,fidelity,constancy\n"+
			"eunis@2021,R22,Species A,diagnostic,0.8,\n"+
			"eunis@2021,R22,Species B,diagnostic,0.8,\n")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"ingest", "--csv-dir", dir, "--db", filepath.Join(t.TempDir(), "situs.sqlite")})

	captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("executing ingest: %v", err)
		}
	})

	if !strings.Contains(out.String(), `"DistributionFailed": 1`) {
		t.Errorf("output = %q, want DistributionFailed to show the one tolerated concept failure", out.String())
	}
	if !strings.Contains(out.String(), `"WithAreas": 1`) {
		t.Errorf("output = %q, want the other concept's area to have been stored despite the failure", out.String())
	}
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
	// WithAreas == 0 must be a visible statement in the printed report, not
	// an absent field an "omitempty" could later drop unnoticed — the stub
	// hostus server here resolves nothing, so the distribution step never
	// finds a concept id and WithAreas is legitimately 0.
	if !strings.Contains(out.String(), `"WithAreas": 0`) {
		t.Errorf("output = %q, want the Distribution report's WithAreas field present", out.String())
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

// SITUS_HOSTUS_ENTRY_BACKBONE is a supported knob, but the prefix the batch
// route accepts is compiled in. Re-pointing the first leaves an index whose
// every id answers unknown_backbone on POST /v1/species/habitat-types while the
// single-concept route keeps working. The run must not fail — an operator may
// mean it — but it must say so.
func TestIngestCommand_WarnsWhenTheIndexBackboneIsNotTheOneTheBatchRouteAccepts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/v1/match" {
			_, _ = w.Write([]byte(`{"results":[{"id":"0","concept_id":"gbif:concept:1","match_type":"exact"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"distribution":[]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SITUS_HOSTUS_BASE_URL", srv.URL)
	t.Setenv("SITUS_HOSTUS_ENTRY_BACKBONE", "gbif")

	dir := seedIngestDir(t)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"ingest", "--csv-dir", dir, "--db", filepath.Join(t.TempDir(), "situs.sqlite")})

	logOutput := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("executing ingest: %v, want the run to succeed despite the mismatch", err)
		}
	})

	if !strings.Contains(logOutput, "batch route cannot answer") {
		t.Errorf("log output = %q, want a warning about the backbone mismatch", logOutput)
	}
	for _, want := range []string{`"index_backbones":["gbif"]`, `"batch_route_backbone":"wcvp"`} {
		if !strings.Contains(logOutput, want) {
			t.Errorf("log output = %q, want it to name %s", logOutput, want)
		}
	}
}

// The counterpart: the ordinary run, where the ingest's backbone and the batch
// route's agree, must stay quiet.
func TestIngestCommand_DoesNotWarnWhenTheBackbonesAgree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/v1/match" {
			_, _ = w.Write([]byte(`{"results":[{"id":"0","concept_id":"wcvp:concept:1","match_type":"exact"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"distribution":[]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SITUS_HOSTUS_BASE_URL", srv.URL)

	dir := seedIngestDir(t)
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"ingest", "--csv-dir", dir, "--db", filepath.Join(t.TempDir(), "situs.sqlite")})

	logOutput := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("executing ingest: %v", err)
		}
	})

	if strings.Contains(logOutput, "batch route cannot answer") {
		t.Errorf("log output = %q, want no backbone warning for an index built on wcvp", logOutput)
	}
}
