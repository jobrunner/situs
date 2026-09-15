package domain

import (
	"fmt"
	"strings"
)

// OriginPattern is one entry of a CORS allow-list: either an exact origin, or a
// wildcard covering the subdomains of a host ("https://*.example.com").
//
// The wildcard applies to the leading host label only. Scheme and port still
// have to match exactly, because widening them would hand responses to servers
// the operator never listed — a plaintext "http://" sibling, or a different
// service on another port of the same host.
type OriginPattern struct {
	origin   Origin
	wildcard bool   // host was written as "*.<suffix>"
	suffix   string // ".example.com" — the part after the "*", wildcard only
}

// ParseOriginPattern parses one allow-list entry. A wildcard entry must carry a
// scheme like any other origin, and the "*" must be a whole leading label:
// "https://*.example.com" is valid, "https://sub*.example.com" is not.
func ParseOriginPattern(s string) (OriginPattern, error) {
	origin, err := ParseOrigin(s)
	if err != nil {
		return OriginPattern{}, err
	}

	if !strings.Contains(origin.Host, "*") {
		return OriginPattern{origin: origin}, nil
	}
	// The base host must be present: "https://*." would leave the suffix ".",
	// which matches any host ending in a dot.
	base, ok := strings.CutPrefix(origin.Host, "*.")
	if !ok || base == "" || strings.Contains(base, "*") {
		return OriginPattern{}, fmt.Errorf(
			"origin pattern %q: \"*\" must be the whole leading host label of a host "+
				"(e.g. https://*.example.com)", s)
	}

	return OriginPattern{
		origin:   origin,
		wildcard: true,
		suffix:   origin.Host[1:], // "*.example.com" -> ".example.com"
	}, nil
}

// MatchesString reports whether a raw Origin header value matches the pattern.
// A malformed origin never matches.
func (p OriginPattern) MatchesString(origin string) bool {
	parsed, err := ParseOrigin(origin)
	if err != nil {
		return false
	}
	return p.Matches(parsed)
}

// Matches reports whether the origin is covered by the pattern.
func (p OriginPattern) Matches(o Origin) bool {
	if o.Scheme != p.origin.Scheme || o.Port != p.origin.Port {
		return false
	}
	if !p.wildcard {
		return o.Host == p.origin.Host
	}
	// Requiring more than the suffix keeps the bare domain out; keeping the
	// leading dot keeps "notexample.com" out.
	return strings.HasSuffix(o.Host, p.suffix) && len(o.Host) > len(p.suffix)
}
