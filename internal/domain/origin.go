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
	// Scheme and DNS host are case-insensitive by spec; the port is a number and
	// has no case, so it is left untouched. Normalizing here means an entry
	// spelled "HTTPS://Example.COM" still matches the lowercase origin a browser
	// actually sends.
	scheme = strings.ToLower(scheme)
	// RFC 3986: scheme = ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ). Checking
	// only "text before ://" accepted junk like "ht/tps" or "https " (a space
	// smuggled in before the "://") — a browser can never send either as the
	// scheme of its Origin header, so such an entry would parse and then never
	// match a real request.
	if !isValidScheme(scheme) {
		return Origin{}, fmt.Errorf("origin %q has an invalid scheme %q", s, scheme)
	}
	if strings.Contains(rest, "/") {
		return Origin{}, fmt.Errorf("origin %q must not contain a path", s)
	}
	// Query and fragment are not part of a serialized origin either — a browser
	// never sends them in the Origin header. An entry carrying one would parse
	// "successfully" and then never match a single real request.
	if strings.Contains(rest, "?") {
		return Origin{}, fmt.Errorf("origin %q must not contain a query (drop everything from \"?\" on)", s)
	}
	if strings.Contains(rest, "#") {
		return Origin{}, fmt.Errorf("origin %q must not contain a fragment (drop everything from \"#\" on)", s)
	}

	host, port, hasPort, err := splitHostPort(rest)
	if err != nil {
		return Origin{}, fmt.Errorf("origin %q: %w", s, err)
	}
	if host == "" {
		return Origin{}, fmt.Errorf("origin %q needs a host", s)
	}
	// A browser never sends userinfo in the Origin header — the header is
	// scheme/host/port only, RFC 6454 §7 does not include it. An entry like
	// "https://user@example.com" would parse "successfully" here and then never
	// match a single real request.
	if strings.Contains(host, "@") {
		return Origin{}, fmt.Errorf("origin %q must not contain userinfo (drop the \"user@\" part — a browser never sends it)", s)
	}
	if hasPort && !isCanonicalPort(port) {
		return Origin{}, fmt.Errorf("origin %q has an invalid port %q (expected 1-65535 in canonical decimal form)", s, port)
	}

	// A browser serializes an explicit default port away: "https://host:443" and
	// "http://host:80" are sent as "https://host" and "http://host" in the
	// Origin header. Normalizing here — not just for https/443 but not for the
	// OTHER scheme's default port ("https://host:80" stays a distinct origin) —
	// means an allow-list entry spelled either way still matches.
	if hasPort && isDefaultPort(scheme, port) {
		port = ""
	}

	return Origin{Scheme: scheme, Host: strings.ToLower(host), Port: port}, nil
}

// isValidScheme reports whether scheme is a syntactically valid URI scheme
// (RFC 3986): a letter, followed by letters, digits, "+", "-" or ".". scheme
// is already lower-cased by the caller, so only the digit/"+"/"-"/"." case
// needs an explicit check here. The caller already rejects an empty scheme
// (strings.Cut's ok is false for one), so an empty string never reaches here.
func isValidScheme(scheme string) bool {
	for i, r := range scheme {
		isLetter := r >= 'a' && r <= 'z'
		isDigitOrSymbol := i > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.')
		if !isLetter && !isDigitOrSymbol {
			return false
		}
	}
	return true
}

// isCanonicalPort reports whether port is both in range (1-65535, the range a
// browser can ever send) and written in the exact decimal form a browser
// would send it in. strconv.Itoa(n) != port catches everything Atoi alone
// lets through that still is not that canonical form: a leading zero
// ("0443"), a leading "+" ("+443"), or leading/trailing space Atoi tolerates.
// Rejecting rather than rewriting "0443" to "443" is more honest — silently
// normalizing would hide a likely typo (a copy-pasted port with a stray
// digit) behind a match that still works.
func isCanonicalPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535 && strconv.Itoa(n) == port
}

// isDefaultPort reports whether port is the scheme's own default — the one a
// browser omits when serializing an origin. It is deliberately scheme-specific:
// "https://host:80" is not https's default and must stay distinct from
// "https://host".
func isDefaultPort(scheme, port string) bool {
	switch scheme {
	case "https":
		return port == "443"
	case "http":
		return port == "80"
	default:
		return false
	}
}

// splitHostPort separates an optional ":port" from a host, leaving a bracketed
// IPv6 literal intact — its colons belong to the address, not to a port.
// hasPort distinguishes "no port given" from a port that is present but empty
// ("example.com:"), which is malformed rather than a default.
func splitHostPort(hostPort string) (host, port string, hasPort bool, err error) {
	if after, found := strings.CutPrefix(hostPort, "["); found {
		literal, rest, closed := strings.Cut(after, "]")
		if !closed {
			return "", "", false, fmt.Errorf("unbalanced %q — an IPv6 literal needs a closing bracket", hostPort)
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
