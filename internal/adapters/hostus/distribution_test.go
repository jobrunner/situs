package hostus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
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
		Areas(context.Background(), []string{"wcvp:concept:1"}); !errors.Is(err, output.ErrResolverUnavailable) {
		t.Errorf("error = %v, want it to wrap output.ErrResolverUnavailable", err)
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
		Areas(context.Background(), []string{"wcvp:concept:1"}); !errors.Is(err, output.ErrResolverUnavailable) {
		t.Errorf("error = %v, want it to wrap output.ErrResolverUnavailable", err)
	}
}

// An http.Client.Timeout fires as an error whose chain contains
// context.DeadlineExceeded even though the caller's ctx never expired. The
// callers (pacedDistributionSource, IngestDistribution) read that chain to
// tell "this run was told to stop" from "one request was slow", so the
// adapter must not hand them a timeout dressed as a deadline.
func TestClient_AreasClientTimeoutIsUnavailabilityNotAnExpiredContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"distribution":[]}`))
	}))
	defer srv.Close()
	defer close(release)

	slow := &http.Client{Transport: srv.Client().Transport, Timeout: 20 * time.Millisecond}
	_, err := NewClient(srv.URL, slow, 50, "wcvp").Areas(context.Background(), []string{"wcvp:concept:1"})
	if !errors.Is(err, output.ErrResolverUnavailable) {
		t.Errorf("error = %v, want it to wrap output.ErrResolverUnavailable", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want it NOT to satisfy errors.Is(context.DeadlineExceeded) while the caller's ctx is alive", err)
	}
}

// The converse: when the caller's own ctx really is done, that IS the news.
// The error must carry it so the caller can abort instead of treating the
// cancellation as one more unavailable upstream.
func TestClient_AreasReportsAnExpiredContextAsSuch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"distribution":[]}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").Areas(ctx, []string{"wcvp:concept:1"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want it to carry context.Canceled", err)
	}
	if errors.Is(err, output.ErrResolverUnavailable) {
		t.Errorf("error = %v, want a canceled run NOT to be reported as an unavailable upstream", err)
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

// When every area of a known concept is filtered out, the key must be absent,
// not present with an empty slice: absence means "unknown to situs", an empty
// slice would wrongly mean "known to not occur anywhere".
func TestClient_AreasOmitsAConceptWhenAllAreasAreFiltered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"distribution":[{"area_scheme":"tdwg_l4","area_code":"GER-OO"}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client(), 50, "wcvp").
		Areas(context.Background(), []string{"wcvp:concept:1"})
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	if _, ok := got["wcvp:concept:1"]; ok {
		t.Errorf("got %v, want the key absent when every area is filtered out", got)
	}
}
