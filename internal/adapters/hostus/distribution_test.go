package hostus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func TestClient_AreasReadsOneConceptPerRequest(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasPrefix(r.URL.Path, "/v1/concept/") {
			t.Errorf("path = %q, want /v1/concept/{id}", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"concept_id":"x","distribution":[
			{"area_scheme":"wgsrpd_l3","area_code":"GER"},
			{"area_scheme":"wgsrpd_l3","area_code":"FRA"}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1", "wcvp:concept:2"})
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("requests = %d, want 2 (hostus has no batch route for distribution)", calls.Load())
	}
	if len(got["wcvp:concept:1"]) != 2 {
		t.Errorf("areas = %v, want two", got["wcvp:concept:1"])
	}
	if got["wcvp:concept:1"][0] != (domain.Area{Scheme: "wgsrpd_l3", Code: "GER"}) {
		t.Errorf("first area = %v, want wgsrpd_l3:GER", got["wcvp:concept:1"][0])
	}
}

// A concept hostus does not know is not an error: it simply has no areas. The
// caller distinguishes "no data" from "does not occur" by absence.
func TestClient_AreasSkipsUnknownConcepts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:404"})
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

func TestClient_AreasReportsAnUnavailableUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1"}); err == nil {
		t.Error("Areas returned nil error on 503; the caller must be able to see the outage")
	}
}

// A control character makes http.NewRequestWithContext itself fail — this
// exercises the request-build error path, distinct from a network or
// upstream-status failure.
func TestClient_AreasReturnsErrorOnMalformedRequestURL(t *testing.T) {
	if _, err := NewClient("http://\x7f", http.DefaultClient, 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1"}); err == nil {
		t.Error("Areas returned nil error on a malformed base URL, want an error")
	}
}

// A closed httptest server is a deterministic way to force the "server
// unreachable" branch, unlike dialing a real socket that may or may not have
// something listening on it.
func TestClient_AreasTransportFailureIsUnavailability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()

	if _, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1"}); err == nil {
		t.Error("Areas returned nil error against an unreachable server, want an error")
	}
}

func TestClient_AreasReturnsErrorOnMalformedResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1"}); err == nil {
		t.Error("Areas returned nil error on a malformed response body, want an error")
	}
}

func TestClient_AreasIgnoresOtherSchemes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"distribution":[
			{"area_scheme":"tdwg_l4","area_code":"GER-OO"},
			{"area_scheme":"wgsrpd_l3","area_code":"GER"}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1"})
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if len(got["wcvp:concept:1"]) != 1 {
		t.Errorf("areas = %v, want only the wgsrpd_l3 one", got["wcvp:concept:1"])
	}
}
