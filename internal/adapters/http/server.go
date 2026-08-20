// Package httpapi is the primary/driving HTTP adapter, built on gorilla/mux.
//
// gorilla/mux is used (not net/http.ServeMux) for named path vars, per-subrouter
// middleware and — decisive here — Router().Walk, which lets the contract test
// enumerate every registered route and compare it with the OpenAPI spec.
//
// The adapter depends only on driving ports (input.*), never on concrete
// application services; the composition root injects them.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/jobrunner/situs/internal/ports/input"
)

// The error-envelope codes, identical to hostus (see the spec's
// Fehlerbehandlung).
const (
	CodeInternalError = "INTERNAL_ERROR"
	CodeInvalidQuery  = "INVALID_QUERY"
	CodeNotFound      = "NOT_FOUND"
)

// Deps are the driving ports the server serves its routes from. The composition
// root injects them; the adapter never constructs an application service.
//
// There is deliberately no name-resolution port here: serving is autark, and a
// nil-able resolver field would be an invitation to reintroduce the runtime
// dependency on hostus.
type Deps struct {
	Health input.HealthChecker
	Query  input.QueryService
}

// Server wraps the HTTP server and its router. It holds only driving ports.
type Server struct {
	server         *http.Server
	router         *mux.Router
	deps           Deps
	logger         *slog.Logger
	serviceName    string
	version        string
	tracerProvider trace.TracerProvider // nil when tracing is disabled
}

// Options carries the optional dependencies.
type Options struct {
	TracerProvider trace.TracerProvider
	ServiceName    string
	Version        string
	// ReadTimeout bounds reading a whole request (config key server.read_timeout).
	// Zero means no limit beyond ReadHeaderTimeout.
	ReadTimeout time.Duration
}

// NewServer builds the server, wires the routes and prepares the http.Server.
func NewServer(addr string, deps Deps, logger *slog.Logger, opts Options) *Server {
	name := opts.ServiceName
	if name == "" {
		name = "situs"
	}
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	s := &Server{
		deps:           deps,
		logger:         logger,
		serviceName:    name,
		version:        version,
		tracerProvider: opts.TracerProvider,
	}
	s.router = s.setupRoutes()
	s.server = &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadTimeout:       opts.ReadTimeout,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// setupRoutes registers every route. Keep it flat and greppable: the contract
// test walks exactly what is registered here, and every route must appear in
// openapi.yaml (both directions are checked).
func (s *Server) setupRoutes() *mux.Router {
	r := mux.NewRouter()

	// Tracing first so later middleware and handlers see the span context.
	// otelmux names spans after the matched route template (low cardinality).
	if s.tracerProvider != nil {
		r.Use(otelmux.Middleware(s.serviceName, otelmux.WithTracerProvider(s.tracerProvider)))
		r.Use(s.traceIDHeaderMiddleware)
	}
	r.Use(s.loggingMiddleware)
	r.Use(s.recoveryMiddleware)

	// Operations surface.
	r.HandleFunc("/health/live", s.handleLiveness).Methods(http.MethodGet)
	r.HandleFunc("/health/ready", s.handleReadiness).Methods(http.MethodGet)
	r.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)
	r.HandleFunc("/openapi", s.handleOpenAPI).Methods(http.MethodGet)
	r.HandleFunc("/docs", s.handleDocs).Methods(http.MethodGet)

	// Versioned read surface.
	r.HandleFunc("/v1/info", s.handleInfo).Methods(http.MethodGet)
	r.HandleFunc("/v1/habitat-type/{typology}/{code}", s.handleHabitatType).Methods(http.MethodGet)
	r.HandleFunc("/v1/habitat-type/{typology}/{code}/species", s.handleHabitatTypeSpecies).Methods(http.MethodGet)
	r.HandleFunc("/v1/species/{conceptId}/habitat-types", s.handleSpeciesHabitatTypes).Methods(http.MethodGet)
	r.HandleFunc("/v1/species/habitat-types", s.handleSpeciesBatch).Methods(http.MethodPost)
	r.HandleFunc("/v1/syntaxon/{id}/habitat-types", s.handleSyntaxonHabitatTypes).Methods(http.MethodGet)

	return r
}

// Router exposes the router so tests and the contract fitness function can walk
// the registered routes.
func (s *Server) Router() *mux.Router { return s.router }

// Start serves until Shutdown is called.
func (s *Server) Start() error { return s.server.ListenAndServe() }

// Shutdown stops the server gracefully.
func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }

// --- handlers ----------------------------------------------------------------

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"service": s.serviceName,
		"version": s.version,
	})
}

// handleLiveness never depends on downstream state: as long as the process can
// serve HTTP, it is alive. Probes answer with a status code and no body.
func (s *Server) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if !s.deps.Health.Ready(r.Context()) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// --- response envelope -------------------------------------------------------

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("encoding response body", "error", err)
	}
}

// writeError writes the single error envelope every handler uses:
// {"error":{"code":"...","message":"..."}} — identical to hostus.
func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// --- middleware --------------------------------------------------------------

func (s *Server) traceIDHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
			w.Header().Set("X-Trace-Id", sc.TraceID().String())
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		s.logger.InfoContext(r.Context(), "request",
			"method", r.Method, "path", r.URL.Path,
			"status", wrapped.statusCode, "duration", time.Since(start))
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				if span := trace.SpanFromContext(r.Context()); span.SpanContext().IsValid() {
					span.RecordError(fmt.Errorf("panic: %v", err), trace.WithStackTrace(true))
					span.SetStatus(otelcodes.Error, "panic recovered")
				}
				s.logger.ErrorContext(r.Context(), "panic recovered", "error", err, "path", r.URL.Path)
				s.writeError(w, http.StatusInternalServerError, CodeInternalError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// responseWriter captures the status code for the logging middleware.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
