package domain

import "testing"

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

func TestOriginPatternMatchesStringRejectsMalformedOrigin(t *testing.T) {
	p, err := ParseOriginPattern("https://example.com")
	if err != nil {
		t.Fatalf("ParseOriginPattern: %v", err)
	}
	if p.MatchesString("not-an-origin") {
		t.Error("a malformed Origin header value must never match")
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
