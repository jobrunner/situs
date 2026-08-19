package hostus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

	got, err := NewClient(srv.URL, srv.Client()).Resolve(
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

	got, err := NewClient(srv.URL, srv.Client()).Resolve(
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

// 1001 names must split into batches of 500/500/1 — the id sent in each
// request is per-batch (0..len(batch)-1), not a global offset. A regression
// to a global id (strconv.Itoa(start+i)) would silently misattribute every
// concept id from the second batch onward, since the server always echoes
// ids starting at 0 within its own view of the batch it received.
func TestClient_ResolveBatchesAt500AndReindexesPerBatch(t *testing.T) {
	const total = 1001
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

	got, err := NewClient(srv.URL, srv.Client()).Resolve(context.Background(), names)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if requests != 3 {
		t.Errorf("requests = %d, want 3 (1001 names batched at 500)", requests)
	}
	if want := []int{500, 500, 1}; !intSlicesEqual(batchSizes, want) {
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

func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestClient_ResolveReturnsErrorOnUpstreamFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on 503; ingest must not silently record every name as unresolvable")
	}
}

func TestClient_ResolveReturnsErrorOnUnparseableResultID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"not-a-number","concept_id":"wcvp:concept:1"}]}`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on an unparseable result id, want an error")
	}
}

func TestClient_ResolveReturnsErrorOnOutOfRangeResultID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"id":"5","concept_id":"wcvp:concept:1"}]}`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on an out-of-range result id, want an error")
	}
}

func TestClient_ResolveReturnsErrorOnMalformedRequestURL(t *testing.T) {
	// A control character makes http.NewRequestWithContext itself fail —
	// this exercises the request-build error path, distinct from a network
	// or upstream-status failure.
	if _, err := NewClient("http://\x7f", http.DefaultClient).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on a malformed base URL, want an error")
	}
}

// A closed httptest server is a deterministic way to force the "server
// unreachable" branch, unlike dialing a real socket that may or may not
// have something listening on it.
func TestClient_ResolveReturnsErrorWhenTheServerIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()

	if _, err := NewClient(srv.URL, srv.Client()).Resolve(context.Background(), []string{"X"}); err == nil {
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

	if _, err := NewClient(srv.URL+"/", srv.Client()).Resolve(context.Background(), []string{"X"}); err != nil {
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

	_, err := NewClient(srv.URL, srv.Client()).Resolve(context.Background(), []string{"X"})
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

	if _, err := NewClient(srv.URL, srv.Client()).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error on a malformed response body, want an error")
	}
}
