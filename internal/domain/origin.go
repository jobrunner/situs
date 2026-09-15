// Origin is a web origin — a scheme/host/port triple — and the rules for
// parsing one out of a CORS allow-list entry.
//
// This belongs in the domain, not in the HTTP adapter: what makes an origin
// valid, and which allow-list entry covers which origin, is a policy with its
// own rules — testable without a request, a router or a port. The adapter
// keeps only the protocol work: read the Origin header, set the response
// headers, answer the preflight.
package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Origin is a web origin: the scheme/host/port triple a browser sends in the
// Origin header. All three parts identify it — two URLs differing in scheme or
// port are different origins, even on the same host.
type Origin struct {
	Scheme string // "https"
	Host   string // "example.com", or a bracketed IPv6 literal "[::1]"
	Port   string // "" when the scheme's default port is used
}

// ParseOrigin parses a concrete origin such as "https://example.com:8443".
//
// It rejects anything that is not a bare origin — a missing scheme, a missing
// host, or a path. A path is not part of an origin; silently dropping one
// would turn "https://*.example.com/private" into a rule covering every
// subdomain.
func ParseOrigin(s string) (Origin, error) {
	// Browsers send "null" for opaque origins: sandboxed iframes, file://
	// documents, some cross-site redirects. Allow-listing it would open the API
	// to any sandboxed document anywhere, and it cannot be narrowed — so it is
	// refused with its own reason; the generic "needs a scheme" advice would
	// suggest the nonsensical "https://null".
	if s == "null" {
		return Origin{}, fmt.Errorf(
			"the opaque origin %q cannot be allow-listed — it would admit any "+
				"sandboxed document; grant the real origin instead", s)
	}

	scheme, rest, ok := strings.Cut(s, "://")
	if !ok || scheme == "" {
		return Origin{}, fmt.Errorf("origin %q needs a scheme — write it as https://%s",
			s, strings.TrimPrefix(s, "://"))
	}
	if strings.Contains(rest, "/") {
		return Origin{}, fmt.Errorf("origin %q must not contain a path", s)
	}

	host, port, hasPort, err := splitHostPort(rest)
	if err != nil {
		return Origin{}, fmt.Errorf("origin %q: %w", s, err)
	}
	if host == "" {
		return Origin{}, fmt.Errorf("origin %q needs a host", s)
	}
	// A port outside 1-65535 is one no browser can ever send, so such an entry
	// would be a rule that silently never matches.
	if hasPort {
		if n, convErr := strconv.Atoi(port); convErr != nil || n < 1 || n > 65535 {
			return Origin{}, fmt.Errorf("origin %q has an invalid port %q (expected 1-65535)", s, port)
		}
	}

	return Origin{Scheme: scheme, Host: host, Port: port}, nil
}

// splitHostPort separates an optional ":port" from a host, leaving a bracketed
// IPv6 literal intact — its colons belong to the address, not to a port.
// hasPort distinguishes "no port given" from a port that is present but empty
// ("example.com:"), which is malformed rather than a default.
func splitHostPort(hostPort string) (host, port string, hasPort bool, err error) {
	if after, found := strings.CutPrefix(hostPort, "["); found {
		literal, rest, closed := strings.Cut(after, "]")
		if !closed {
			return hostPort, "", false, nil // unbalanced: treat the whole thing as the host
		}
		// Only ":port" may follow the literal. Anything else is neither host nor
		// port; letting it through would leave an entry no browser origin can
		// ever match.
		if rest != "" && !strings.HasPrefix(rest, ":") {
			return "", "", false, fmt.Errorf("unexpected %q after the IPv6 literal (expected \":port\" or nothing)", rest)
		}
		p, hasP := strings.CutPrefix(rest, ":")
		return "[" + literal + "]", p, hasP, nil
	}

	if h, p, found := strings.Cut(hostPort, ":"); found {
		return h, p, true, nil
	}
	return hostPort, "", false, nil
}
