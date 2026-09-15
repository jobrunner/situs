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
