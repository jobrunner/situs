// Optional CORS for the HTTP adapter.
//
// CORS stays off unless SITUS_SERVER_CORS_ALLOWED_ORIGINS names at least one
// origin, so a service whose only browser client is the same-origin explorer
// under "/" keeps byte-identical responses.
//
// READ THIS BEFORE "SIMPLIFYING" IT INTO router.Use:
// gorilla/mux runs Use-middleware only for requests that MATCH a route. An
// OPTIONS preflight against a route registered as .Methods(GET) (or POST)
// matches nothing and goes to the MethodNotAllowedHandler — outside the
// middleware chain. Registered with router.Use, CORS therefore answers every
// preflight with a bare 405 and no CORS headers, and any endpoint that
// triggers a preflight is unusable from a browser. Measured on gorilla/mux
// v1.8.1: Use-middleware -> 405, no headers; outer wrapper -> 204, headers
// present. situs has two POST routes today (/v1/species/habitat-types,
// /v1/species/traits/summary) for which this is real, not theoretical.
//
// The trap is that a GET-only API without auth never sends a preflight, so the
// bug stays invisible until the first endpoint takes a JSON body. Pin it with
// a test that fires OPTIONS at a POST route and asserts 204 plus
// Access-Control-Allow-Origin.
//
// Two neighboring "fixes" for the 405 above are worse than the bug, both
// measured on gorilla/mux v1.8.1:
//   - adding http.MethodOptions to the route's .Methods(...) makes the
//     preflight MATCH — and it is then dispatched to the business handler.
//     Probe against a POST route: the preflight would execute the write.
//     Allow-Origin is still unset, so the browser discards the answer and the
//     caller never learns that a write happened.
//   - mux.CORSMethodMiddleware(r) sets Access-Control-Allow-Methods and
//     nothing else — never Allow-Origin — and only once an OPTIONS route
//     matches. It advertises methods; it does not implement CORS. Combined
//     with the previous point it reads like a complete solution and is the
//     write-on-preflight bug with a helper on top.
//
// Two more things worth testing, both easy to get wrong:
//   - a wildcard must not leak across scheme or port ("https://*.example.com"
//     admitting "http://sub.example.com" hands responses to a plaintext
//     origin);
//   - a bare OPTIONS without Origin must still reach the router, or enabling
//     CORS silently turns every OPTIONS into a 204.
package httpapi

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/jobrunner/situs/internal/domain"
)

// corsMaxAgeSeconds is how long a browser may cache a preflight result.
const corsMaxAgeSeconds = "86400" // 24 hours

// initCORS parses the configured allow-list once, at startup. Patterns that
// cannot be parsed are reported rather than skipped in silence: an entry with
// a typo (or without a scheme) matches nothing, and an operator who never sees
// a message is left believing CORS is configured when it is not. Parsing here
// also keeps the request path a comparison rather than a re-parse of the whole
// list.
func (s *Server) initCORS(origins []string) {
	for _, raw := range origins {
		p, err := domain.ParseOriginPattern(raw)
		if err != nil {
			s.logger.Warn("ignoring unusable CORS origin pattern", "pattern", raw, "error", err)
			continue
		}
		s.corsPatterns = append(s.corsPatterns, p)
	}

	// Each bad entry already logged its own Warn above, but that leaves the
	// consequence unstated: if every entry was unusable, CORS ends up fully
	// disabled even though the operator configured it — the same trap the
	// per-entry warning guards against, one level up. Error, not Warn: unlike
	// a single bad entry in an otherwise-working list, this is not a partial
	// degradation an operator might reasonably miss — the whole feature they
	// asked for silently never turns on, and only this line says so.
	if len(origins) > 0 && len(s.corsPatterns) == 0 {
		s.logger.Error("CORS was configured but every allowed-origin entry was unusable — CORS stays disabled",
			"origins", origins)
	}
}

// wrapCORS returns h unchanged when no usable origins are configured, so a
// service without CORS pays nothing per request.
func (s *Server) wrapCORS(h http.Handler) http.Handler {
	if len(s.corsPatterns) == 0 {
		return h
	}
	return s.corsHandler(h)
}

// corsHandler wraps the whole router — deliberately NOT registered via
// router.Use; see the file header for why.
func (s *Server) corsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// The response depends on Origin whether or not this particular request
		// carries one: a shared cache (proxy, CDN) that stored a response to a
		// request WITHOUT Origin — under a key with no Vary: Origin — could
		// otherwise hand that same cached response to a later request that DOES
		// carry an allowed Origin, and the caller gets a response with no
		// Access-Control-Allow-Origin even though its origin is allowed. Setting
		// Vary unconditionally closes that gap; Add (not Set) avoids clobbering a
		// Vary value the handler chain sets further down.
		w.Header().Add("Vary", "Origin")
		requestedMethod := r.Header.Get("Access-Control-Request-Method")
		if requestedMethod != "" {
			// A preflight response also depends on the requested method — the
			// allowed methods are derived from it — so two preflights from the
			// same origin asking about different methods must not share a cache
			// entry either.
			w.Header().Add("Vary", "Access-Control-Request-Method")
		}
		requestedHeaders := r.Header.Get("Access-Control-Request-Headers")
		if requestedHeaders != "" {
			// Allow-Headers below is derived from this, so — same reasoning as
			// Access-Control-Request-Method above — two preflights from the same
			// origin asking about different headers must not share a cache entry.
			w.Header().Add("Vary", "Access-Control-Request-Headers")
		}
		if origin != "" && s.isOriginAllowed(origin) {
			s.setAllowedOriginHeaders(w, r, origin, requestedMethod, requestedHeaders)
		}

		// Only a real preflight is short-circuited: OPTIONS carrying both Origin
		// and Access-Control-Request-Method. A bare OPTIONS keeps falling
		// through to the router exactly as it did before CORS existed —
		// otherwise enabling CORS would silently turn every OPTIONS into a 204.
		//
		// Answering 204 regardless of whether the origin is *allowed* is
		// deliberate: without the Allow-Origin header above the browser rejects
		// the response anyway, and a uniform answer avoids leaking which
		// origins are configured.
		if r.Method == http.MethodOptions && origin != "" && requestedMethod != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// setAllowedOriginHeaders sets the headers that only make sense once origin is
// known to be on the allow-list. Split out of corsHandler to keep that
// function's branching shallow — this is the part that grows whenever a new
// preflight header (Allow-Methods, Allow-Headers, ...) is added.
func (s *Server) setAllowedOriginHeaders(w http.ResponseWriter, r *http.Request, origin, requestedMethod, requestedHeaders string) {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	// Advertise the method the ROUTER actually accepts for this path, rather
	// than a hand-written list. A literal "GET, POST, OPTIONS" here is correct
	// exactly until someone adds a route with a new method: nothing fails at
	// build time, and the endpoint is simply unusable from a browser. Deriving
	// the answer from the route table makes that drift impossible.
	if requestedMethod != "" && s.routeAllowsMethod(r, requestedMethod) {
		w.Header().Set("Access-Control-Allow-Methods", requestedMethod+", OPTIONS")
	}
	// Mirror back exactly the headers the browser asked about, rather than a
	// hand-written allow-list. situs has no login and so no header worth
	// denying: a fixed list ("Accept, Content-Type, Authorization") rejects
	// any caller sending something else — say X-Request-Id — with nothing
	// anywhere saying why. Reflecting the request is the standard shape for a
	// service with no credentials and no header allow-list to enforce.
	if requestedHeaders != "" {
		w.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
	}
	w.Header().Set("Access-Control-Max-Age", corsMaxAgeSeconds)
}

// routeAllowsMethod asks the router whether method+path would match a route.
// mux reports a path that exists under a different method via
// RouteMatch.MatchErr == ErrMethodMismatch, so a method the service does not
// serve is never advertised as allowed.
func (s *Server) routeAllowsMethod(r *http.Request, method string) bool {
	probe := r.Clone(r.Context())
	probe.Method = method
	var match mux.RouteMatch
	return s.router.Match(probe, &match) && match.MatchErr == nil
}

// isOriginAllowed reports whether the request's Origin matches any configured
// pattern. Matching itself is domain logic (see internal/domain/originpattern.go);
// the adapter only asks.
func (s *Server) isOriginAllowed(origin string) bool {
	for _, p := range s.corsPatterns {
		if p.MatchesString(origin) {
			return true
		}
	}
	return false
}
