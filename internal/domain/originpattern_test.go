package domain

import (
	"strings"
	"testing"
)

func TestParseOriginPattern(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{in: "https://example.com"},
		{in: "https://*.example.com"},
		{in: "http://localhost:5173"},

		{in: "not-an-origin", wantErr: true},
		{in: "https://*", wantErr: true}, // "*" not a whole label with a base host
		{in: "https://sub*.example.com", wantErr: true},
		{in: "https://*.example.*.com", wantErr: true},
		{in: "https://*.", wantErr: true},
	}
	for _, tc := range tests {
		_, err := ParseOriginPattern(tc.in)
		if tc.wantErr && err == nil {
			t.Errorf("ParseOriginPattern(%q) = nil error, want error", tc.in)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ParseOriginPattern(%q): unexpected error %v", tc.in, err)
		}
	}
}

// TestParseOriginPatternBareWildcardGivesActionableAdvice pins the fix for a
// dead-end error: the bare "*" must not be told to write itself as "https://*"
// — that rewritten form is rejected too, by the wildcard-label check. The
// message must point at a suffix that actually works.
func TestParseOriginPatternBareWildcardGivesActionableAdvice(t *testing.T) {
	_, err := ParseOriginPattern("*")
	if err == nil {
		t.Fatal("ParseOriginPattern(\"*\") = nil error, want error")
	}
	if strings.Contains(err.Error(), "https://*\"") {
		t.Errorf("error %q still advises the bare https://*, which is itself rejected", err.Error())
	}
	// The suggested rewrite must itself parse.
	if _, rewriteErr := ParseOriginPattern("https://*.example.com"); rewriteErr != nil {
		t.Fatalf("the suggested rewrite https://*.example.com must itself be valid: %v", rewriteErr)
	}
}

func TestOriginPatternMatchesExactOrigin(t *testing.T) {
	p, err := ParseOriginPattern("https://example.com:8443")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if !p.MatchesString("https://example.com:8443") {
		t.Error("exact origin should match")
	}
	if p.MatchesString("https://example.com") {
		t.Error("origin with a different port must not match")
	}
	if p.MatchesString("http://example.com:8443") {
		t.Error("origin with a different scheme must not match")
	}
	if p.MatchesString("https://other.com:8443") {
		t.Error("origin with a different host must not match")
	}
}

func TestOriginPatternMatchesWildcardSubdomain(t *testing.T) {
	p, err := ParseOriginPattern("https://*.fieldworksdiary.app")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}

	cases := []struct {
		origin string
		want   bool
	}{
		{"https://app.fieldworksdiary.app", true},
		{"https://a.b.fieldworksdiary.app", true},
		{"https://fieldworksdiary.app", false},          // bare domain, no subdomain
		{"https://notfieldworksdiary.app", false},       // suffix without the dot must not match
		{"http://app.fieldworksdiary.app", false},       // scheme must match exactly
		{"https://app.fieldworksdiary.app:8443", false}, // port must match exactly
	}
	for _, tc := range cases {
		if got := p.MatchesString(tc.origin); got != tc.want {
			t.Errorf("MatchesString(%q) = %v, want %v", tc.origin, got, tc.want)
		}
	}
}

// TestOriginPatternWildcardRequiresMoreThanTheSuffix pins the ">" in
// len(o.Host) > len(p.suffix): a host exactly equal to the suffix (i.e. no
// actual subdomain label in front of it) must not match, only one strictly
// longer than the suffix.
func TestOriginPatternWildcardRequiresMoreThanTheSuffix(t *testing.T) {
	p, err := ParseOriginPattern("https://*.fieldworksdiary.app")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if p.MatchesString("https://.fieldworksdiary.app") {
		t.Error("a host equal to the wildcard suffix (no real subdomain) must not match")
	}
}

// TestOriginPatternMatchesDefaultPortEitherWay pins the default-port
// normalization: an allow-list entry with an explicit ":443" (or ":80") must
// match a request whose Origin header omits it, and vice versa — a browser
// never sends the default port in the Origin header.
func TestOriginPatternMatchesDefaultPortEitherWay(t *testing.T) {
	withPort, err := ParseOriginPattern("https://example.com:443")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if !withPort.MatchesString("https://example.com") {
		t.Error("pattern with explicit :443 should match an Origin header without a port")
	}

	withoutPort, err := ParseOriginPattern("https://example.com")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if !withoutPort.MatchesString("https://example.com:443") {
		t.Error("pattern without a port should match an Origin header with explicit :443")
	}
}

// TestOriginPatternDoesNotNormalizeTheOtherSchemesDefaultPort pins the
// boundary: ":80" is http's default, not https's, so "https://example.com:80"
// must stay a distinct origin from "https://example.com".
func TestOriginPatternDoesNotNormalizeTheOtherSchemesDefaultPort(t *testing.T) {
	p, err := ParseOriginPattern("https://example.com:80")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if p.MatchesString("https://example.com") {
		t.Error("https://example.com:80 must not match https://example.com — :80 is not https's default port")
	}
}

func TestOriginPatternMatchesStringRejectsMalformedOrigin(t *testing.T) {
	p, err := ParseOriginPattern("https://example.com")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if p.MatchesString("not-an-origin") {
		t.Error("a malformed Origin header value must never match")
	}
}

// TestOriginPatternMatchesStringIsCaseInsensitiveInSchemeAndHost pins the fix
// for an allow-list entry spelled with any case: scheme and DNS host are
// case-insensitive, so "HTTPS://Example.COM" must still match the lowercase
// origin a browser actually sends — and a literally lowercase entry must keep
// working exactly as before.
func TestOriginPatternMatchesStringIsCaseInsensitiveInSchemeAndHost(t *testing.T) {
	upper, err := ParseOriginPattern("HTTPS://Example.COM")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if !upper.MatchesString("https://example.com") {
		t.Error("an upper-cased allow-list entry must match the lowercase origin a browser sends")
	}

	lower, err := ParseOriginPattern("https://example.com")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if !lower.MatchesString("https://example.com") {
		t.Error("a literally lowercase allow-list entry must still match")
	}
}

func TestOriginPatternMatchesAcceptsOriginValue(t *testing.T) {
	p, err := ParseOriginPattern("https://example.com")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if !p.Matches(Origin{Scheme: "https", Host: "example.com"}) {
		t.Error("Matches should accept an already-parsed Origin equal to the pattern")
	}
}

// TestParseOriginPatternTrimsSurroundingWhitespace pins the fix for the most
// common shape of the recurring bug class: SITUS_SERVER_CORS_ALLOWED_ORIGINS
// is comma-separated, and writing "a, b" — a space after the comma — is the
// natural way to write such a list. Left untrimmed, the space becomes part of
// the scheme or host, the entry is accepted, and it can never equal a real
// Origin header, which never carries whitespace.
func TestParseOriginPatternTrimsSurroundingWhitespace(t *testing.T) {
	p, err := ParseOriginPattern(" https://leer.de ")
	if err != nil {
		t.Fatalf("ParseOriginPattern(%q): %v", " https://leer.de ", err)
	}
	if !p.MatchesString("https://leer.de") {
		t.Error("a pattern with leading/trailing whitespace must match the trimmed origin")
	}

	tab, err := ParseOriginPattern("\thttps://tab.example.com\n")
	if err != nil {
		t.Fatalf("ParseOriginPattern with tab/newline whitespace: %v", err)
	}
	if !tab.MatchesString("https://tab.example.com") {
		t.Error("a pattern with tab/newline whitespace must match the trimmed origin")
	}
}

// TestParseOriginPatternRejectsBlankEntry pins the decision for a comma list
// like "a,,b" or a trailing "a,": the empty entry produced by the split must
// be reported, not silently dropped — silently dropping it would look exactly
// like the bug it replaces (an entry accepted at startup that can never be
// hit), just one step earlier, at the split instead of the parse.
func TestParseOriginPatternRejectsBlankEntry(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n"} {
		if _, err := ParseOriginPattern(in); err == nil {
			t.Errorf("ParseOriginPattern(%q) = nil error, want error for a blank entry", in)
		}
	}
}

// TestParseOriginPatternRejectsInvalidScheme pins the fix for an entry whose
// scheme merely contains "://" somewhere with non-empty text in front of it,
// without being a scheme a browser could ever send.
func TestParseOriginPatternRejectsInvalidScheme(t *testing.T) {
	for _, in := range []string{"ht/tps://kaputt.de", "https ://example.com", "1https://example.com"} {
		if _, err := ParseOriginPattern(in); err == nil {
			t.Errorf("ParseOriginPattern(%q) = nil error, want error for an invalid scheme", in)
		}
	}
}

// TestParseOriginPatternRejectsNonCanonicalPort pins the fix for a port kept
// as a string despite passing through Atoi: "0443" and "+443" both convert to
// the port number 443, but a browser never sends a leading zero or a leading
// "+" in the Origin header's port, so such an entry would never match.
func TestParseOriginPatternRejectsNonCanonicalPort(t *testing.T) {
	for _, in := range []string{"https://example.com:0443", "https://example.com:+443", "https://example.com:04"} {
		if _, err := ParseOriginPattern(in); err == nil {
			t.Errorf("ParseOriginPattern(%q) = nil error, want error for a non-canonical port", in)
		}
	}
}

// TestEveryAcceptedOriginPatternMatchesItsOwnCanonicalOrigin is the
// class-level guard, not another regression pin: every one of the seven CORS
// review rounds was the same shape — an entry the parser accepted, whose
// canonical form (what MatchesString ultimately compares against) was NOT the
// string a browser could ever send in its Origin header. Instead of adding an
// eighth pinned case when the next variant is found, this test derives the
// browser-sent string mechanically — scheme://host[:port] — from the accepted
// pattern itself and asserts the pattern matches it. A future bug of this
// exact shape (parser accepts X, but no real Origin header ever equals X's
// canonical serialization) fails this test without a new case being added,
// as long as the bad entry is admitted by ParseOrigin/ParseOriginPattern.
//
// What this does NOT catch: a bug where the canonical serialization below is
// itself wrong in a way that happens to agree with Matches (e.g. both the
// serializer and Matches independently mis-handle IDN/punycode hosts the same
// way), or a bug confined to the wildcard-suffix arm that no non-wildcard
// entry exercises — see the wildcard-specific sub-test below for that arm.
func TestEveryAcceptedOriginPatternMatchesItsOwnCanonicalOrigin(t *testing.T) {
	entries := []string{
		"https://example.com",
		"https://example.com:8443",
		"http://localhost:5173",
		"https://[::1]:8443",
		"https://[::1]",
		"HTTPS://Example.COM",
		"HtTp://LocalHost:5173",
		"https://example.com:1",
		"https://example.com:65535",
		"https://example.com:443",
		"http://example.com:80",
		"https://example.com:80",
		"http://example.com:443",
		"ftp://example.com:21",
		"  https://spaced.example.com  ",
		"https://*.example.com",
	}

	for _, raw := range entries {
		p, err := ParseOriginPattern(raw)
		if err != nil {
			t.Fatalf("ParseOriginPattern(%q): unexpected error %v", raw, err)
		}

		origin, err := ParseOrigin(strings.TrimSpace(raw))
		if err != nil {
			// A wildcard entry ("*.example.com" in the host) is not itself a
			// browser origin; substitute a concrete subdomain instead.
			if !strings.Contains(raw, "*") {
				t.Fatalf("ParseOrigin(%q): unexpected error %v", raw, err)
			}
			concrete := strings.Replace(strings.TrimSpace(raw), "*", "sub", 1)
			origin, err = ParseOrigin(concrete)
			if err != nil {
				t.Fatalf("ParseOrigin(%q): unexpected error %v", concrete, err)
			}
		}

		// canonicalOrigin is exactly what a browser sends for this origin:
		// scheme://host[:port], nothing else.
		browserSent := origin.Scheme + "://" + origin.Host
		if origin.Port != "" {
			browserSent += ":" + origin.Port
		}

		if !p.MatchesString(browserSent) {
			t.Errorf("ParseOriginPattern(%q) accepted, but its own canonical origin %q is never matched — "+
				"this pattern can never be hit by a real browser request", raw, browserSent)
		}
	}
}

// TestEveryAcceptedWildcardMatchesAConcreteBrowserOrigin exercises the
// wildcard-suffix arm of Matches specifically: it is not reachable from
// TestEveryAcceptedOriginPatternMatchesItsOwnCanonicalOrigin's own-origin
// round trip above (a pattern's "canonical origin" for a wildcard entry is
// itself a substitution, not a derivation from Matches' suffix logic), so it
// is asserted directly here.
func TestEveryAcceptedWildcardMatchesAConcreteBrowserOrigin(t *testing.T) {
	entries := []string{
		"https://*.example.com",
		"http://*.internal.example.org:8080",
		" https://*.spaced.example.com ",
	}
	for _, raw := range entries {
		p, err := ParseOriginPattern(raw)
		if err != nil {
			t.Fatalf("ParseOriginPattern(%q): unexpected error %v", raw, err)
		}
		concrete := strings.Replace(strings.TrimSpace(raw), "*", "sub", 1)
		if !p.MatchesString(concrete) {
			t.Errorf("wildcard pattern %q accepted, but a concrete subdomain %q it should cover is never matched",
				raw, concrete)
		}
	}
}
