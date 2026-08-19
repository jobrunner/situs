package hostus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestClient_ResolveReturnsErrorWhenTheServerIsUnreachable(t *testing.T) {
	if _, err := NewClient("http://127.0.0.1:1", http.DefaultClient).Resolve(context.Background(), []string{"X"}); err == nil {
		t.Error("Resolve returned nil error against an unreachable server, want an error")
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
