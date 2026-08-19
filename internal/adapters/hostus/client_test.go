package hostus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/ports/output"
)

// quietSlog keeps the downshift's warnings out of the test output: they are
// behavior under test, not something a reader of a green run needs to see.
func quietSlog(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(previous) })
}

func TestClient_ResolveMapsVerbatimToConceptID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/match" {
			t.Errorf("path = %q, want /v1/match", r.URL.Path)
		}
		var req struct {
			Names []struct {
				ID       string `json:"id"`
				Verbatim string `json:"verbatim"`
			} `json:"names"`
			EntryBackbone string `json:"entry_backbone"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if len(req.Names) != 2 {
			t.Errorf("names = %d, want 2 (batched in one call)", len(req.Names))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"0","concept_id":"wcvp:concept:1","match_type":"exact"},
			{"id":"1","match_type":"unresolvable"}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(
		context.Background(), []string{"Inula hirta", "Nonexistent name"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["Inula hirta"] != "wcvp:concept:1" {
		t.Errorf("resolved = %q, want wcvp:concept:1", got["Inula hirta"])
	}
	if _, ok := got["Nonexistent name"]; ok {
		t.Error("unresolvable name must be absent from the map, not empty-valued")
	}
}

// A response that returns results in an order different from the sent
// batch pins that mapping is done by the echoed id, never by array
// position — an implementation that indexed batch[i] by response position
// would pass the happy-path test above identically but fail this one.
func TestClient_ResolveMapsByIDNotByResponsePosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"id":"1","concept_id":"wcvp:concept:second"},
			{"id":"0","concept_id":"wcvp:concept:first"}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(
		context.Background(), []string{"First name", "Second name"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got["First name"] != "wcvp:concept:first" {
		t.Errorf("First name = %q, want wcvp:concept:first (reversed response order)", got["First name"])
	}
	if got["Second name"] != "wcvp:concept:second" {
		t.Errorf("Second name = %q, want wcvp:concept:second (reversed response order)", got["Second name"])
	}
}

// A name list longer than one batch must split at batchSize — the id sent in each
// request is per-batch (0..len(batch)-1), not a global offset. A regression
// to a global id (strconv.Itoa(start+i)) would silently misattribute every
// concept id from the second batch onward, since the server always echoes
// ids starting at 0 within its own view of the batch it received.
func TestClient_ResolveBatchesAndReindexesPerBatch(t *testing.T) {
	const total = 2*DefaultBatchSize + 1
	names := make([]string, total)
	for i := range names {
		names[i] = fmt.Sprintf("Species %d", i)
	}

	var requests int
	var batchSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var req struct {
			Names []struct {
				ID       string `json:"id"`
				Verbatim string `json:"verbatim"`
			} `json:"names"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		batchSizes = append(batchSizes, len(req.Names))

		results := make([]map[string]string, len(req.Names))
		for i, n := range req.Names {
			// Echo the per-batch id back, distinguishing concept ids by the
			// verbatim name so a misattribution shows up as a mismatch.
			results[i] = map[string]string{"id": n.ID, "concept_id": "concept:" + n.Verbatim}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), names)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if requests != 3 {
		t.Errorf("requests = %d, want 3 (%d names batched at %d)", requests, total, DefaultBatchSize)
	}
	if want := []int{DefaultBatchSize, DefaultBatchSize, 1}; !slices.Equal(batchSizes, want) {
		t.Errorf("batch sizes = %v, want %v", batchSizes, want)
	}
	if len(got) != total {
		t.Fatalf("resolved %d names, want %d (none dropped, none duplicated)", len(got), total)
	}
	for _, n := range names {
		if got[n] != "concept:"+n {
			t.Errorf("resolved[%q] = %q, want %q", n, got[n], "concept:"+n)
		}
	}
}

func TestClient_ResolveReturnsErrorOnUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on 503; ingest must not silently record every name as unresolvable")
	}
}

func TestClient_ResolveReturnsErrorOnUnparseableResultID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"not-a-number","concept_id":"wcvp:concept:1"}]}`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on an unparseable result id, want an error")
	}
}

func TestClient_ResolveReturnsErrorOnOutOfRangeResultID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"5","concept_id":"wcvp:concept:1"}]}`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on an out-of-range result id, want an error")
	}
}

func TestClient_ResolveReturnsErrorOnMalformedRequestURL(t *testing.T) {
	// A control character makes http.NewRequestWithContext itself fail —
	// this exercises the request-build error path, distinct from a network
	// or upstream-status failure.
	if _, err := NewClient("http://\x7f", http.DefaultClient, DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on a malformed base URL, want an error")
	}
}

// A closed httptest server is a deterministic way to force the "server
// unreachable" branch, unlike dialing a real socket that may or may not
// have something listening on it.
func TestClient_ResolveReturnsErrorWhenTheServerIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()

	if _, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error against an unreachable server, want an error")
	}
}

func TestNewClient_TrimsATrailingSlashFromBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL+"/", srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if gotPath != "/v1/match" {
		t.Errorf("path = %q, want /v1/match (a trailing slash in baseURL must not produce //v1/match)", gotPath)
	}
}

func TestClient_ResolveErrorIncludesABoundedResponseBodySnippet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unknown entry_backbone"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"})
	if err == nil {
		t.Fatal("Resolve returned nil error on a 400, want an error")
	}
	if !strings.Contains(err.Error(), "unknown entry_backbone") {
		t.Errorf("error = %q, want it to include the response body", err)
	}
}

func TestClient_ResolveReturnsErrorOnMalformedResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on a malformed response body, want an error")
	}
}

// The default is not merely "whatever the constant says": it was measured
// against hostus' fixed 30s per-request timeout (worst 50-name window 16.3s,
// worst 100-name window 19.5s, 500 names exceeded it). A regression to a larger
// default would break the ingest on real data, so the ceiling is pinned here
// rather than derived from the constant under test.
func TestDefaultBatchSize_StaysAtOrBelowTheMeasuredCeiling(t *testing.T) {
	const measuredCeiling = 50
	if DefaultBatchSize > measuredCeiling {
		t.Errorf("DefaultBatchSize = %d, want <= %d (measured against hostus' fixed 30s request timeout)",
			DefaultBatchSize, measuredCeiling)
	}
	if DefaultBatchSize <= minBatchSize {
		t.Errorf("DefaultBatchSize = %d, want more than the downshift floor %d", DefaultBatchSize, minBatchSize)
	}
}

func TestNewClient_BatchSizeIsConfigurableAndFallsBackToTheDefault(t *testing.T) {
	var batchSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Names []struct{} `json:"names"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		batchSizes = append(batchSizes, len(req.Names))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	names := []string{"a", "b", "c", "d", "e"}
	if _, err := NewClient(srv.URL, srv.Client(), 2, DefaultEntryBackbone).Resolve(context.Background(), names); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := []int{2, 2, 1}; !slices.Equal(batchSizes, want) {
		t.Errorf("batch sizes = %v, want %v (the configured size must be used)", batchSizes, want)
	}

	batchSizes = nil
	if _, err := NewClient(srv.URL, srv.Client(), 0, DefaultEntryBackbone).Resolve(context.Background(), names); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := []int{len(names)}; !slices.Equal(batchSizes, want) {
		t.Errorf("batch sizes = %v, want %v (a size <= 0 must fall back to the default)", batchSizes, want)
	}
}

// The entry backbone is a hostus parameter like base_url, timeout and
// batch_size: a hostus index built on another backbone must be reachable by
// configuration, not by recompiling. An empty value falls back to the default.
func TestClient_SendsTheConfiguredEntryBackbone(t *testing.T) {
	for name, tc := range map[string]struct{ configured, want string }{
		"configured":                      {configured: "gbif", want: "gbif"},
		"empty falls back to the default": {configured: "", want: DefaultEntryBackbone},
	} {
		t.Run(name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req matchRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("decoding request: %v", err)
					return
				}
				got = req.EntryBackbone
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"results":[]}`))
			}))
			defer srv.Close()

			if _, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, tc.configured).
				Resolve(context.Background(), []string{"Inula hirta"}); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != tc.want {
				t.Errorf("entry_backbone = %q, want %q", got, tc.want)
			}
		})
	}
}

// The two failure modes point at different systems and must be distinguishable:
// a resolver that does not answer is an outage, a resolver that answers "your
// request is wrong" is a fault on this side.
func TestClient_ResolveDistinguishesUnavailableFromRejected(t *testing.T) {
	for name, tc := range map[string]struct {
		status  int
		want    error
		wantNot error
	}{
		"400 is a rejection":    {status: http.StatusBadRequest, want: output.ErrResolverRejected, wantNot: output.ErrResolverUnavailable},
		"404 is a rejection":    {status: http.StatusNotFound, want: output.ErrResolverRejected, wantNot: output.ErrResolverUnavailable},
		"500 is unavailability": {status: http.StatusInternalServerError, want: output.ErrResolverUnavailable, wantNot: output.ErrResolverRejected},
		"503 is unavailability": {status: http.StatusServiceUnavailable, want: output.ErrResolverUnavailable, wantNot: output.ErrResolverRejected},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			_, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"})
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want it to wrap %v", err, tc.want)
			}
			if errors.Is(err, tc.wantNot) {
				t.Errorf("error = %v, must not also wrap %v — the two failure modes name different systems", err, tc.wantNot)
			}
		})
	}
}

// A transport failure is the resolver not answering at all.
func TestClient_TransportFailureIsUnavailability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()

	_, err := NewClient(srv.URL, srv.Client(), DefaultBatchSize, DefaultEntryBackbone).Resolve(context.Background(), []string{"X"})
	if !errors.Is(err, output.ErrResolverUnavailable) {
		t.Errorf("error = %v, want it to wrap output.ErrResolverUnavailable", err)
	}
}

// The configured batch size is a default, not a cliff: hostus' per-request
// timeout is fixed and the cost of a batch depends on its content, so a batch
// that is too large for this machine must be retried smaller instead of failing
// a 13791-row ingest outright.
func TestClient_DownshiftsTheBatchWhenTheResolverCannotAnswerIt(t *testing.T) {
	quietSlog(t)

	const answerable = 10
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Names []struct {
				ID       string `json:"id"`
				Verbatim string `json:"verbatim"`
			} `json:"names"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		sizes = append(sizes, len(req.Names))
		// Stand-in for hostus running out of time on a large batch.
		if len(req.Names) > answerable {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		results := make([]map[string]string, len(req.Names))
		for i, n := range req.Names {
			results[i] = map[string]string{"id": n.ID, "concept_id": "concept:" + n.Verbatim}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer srv.Close()

	names := make([]string, 40)
	for i := range names {
		names[i] = fmt.Sprintf("Species %d", i)
	}

	got, err := NewClient(srv.URL, srv.Client(), 40, DefaultEntryBackbone).Resolve(context.Background(), names)
	if err != nil {
		t.Fatalf("Resolve = %v, want the downshift to carry the ingest through", err)
	}
	if len(got) != len(names) {
		t.Errorf("resolved %d of %d names — the downshift must not drop any", len(got), len(names))
	}
	for _, n := range names {
		if got[n] != "concept:"+n {
			t.Errorf("resolved[%q] = %q, want %q (ids must stay per-batch after a downshift)", n, got[n], "concept:"+n)
		}
	}
	if len(sizes) < 2 || sizes[0] != 40 {
		t.Fatalf("request sizes = %v, want the first attempt at 40 followed by smaller ones", sizes)
	}
	for _, s := range sizes[1:] {
		if s > 40 {
			t.Errorf("request sizes = %v, want every retry smaller than the first attempt", sizes)
		}
	}
}

// A rejection is deterministic: retrying it smaller would only multiply the
// same error.
func TestClient_DoesNotDownshiftARejectedRequest(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	names := make([]string, 20)
	for i := range names {
		names[i] = fmt.Sprintf("Species %d", i)
	}

	_, err := NewClient(srv.URL, srv.Client(), 20, DefaultEntryBackbone).Resolve(context.Background(), names)
	if !errors.Is(err, output.ErrResolverRejected) {
		t.Fatalf("error = %v, want it to wrap output.ErrResolverRejected", err)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want exactly 1 — a rejection must not be retried", requests)
	}
}

// The downshift has a floor: an unavailable resolver must fail, not halve
// forever.
func TestClient_DownshiftStopsAtTheFloor(t *testing.T) {
	quietSlog(t)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	names := make([]string, 20)
	for i := range names {
		names[i] = fmt.Sprintf("Species %d", i)
	}

	_, err := NewClient(srv.URL, srv.Client(), 20, DefaultEntryBackbone).Resolve(context.Background(), names)
	if !errors.Is(err, output.ErrResolverUnavailable) {
		t.Fatalf("error = %v, want it to wrap output.ErrResolverUnavailable", err)
	}
	if requests > 4 {
		t.Errorf("requests = %d, want the halving to stop at the floor (%d), not to keep retrying",
			requests, minBatchSize)
	}
}
