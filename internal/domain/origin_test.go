package domain

import "testing"

func TestParseOrigin(t *testing.T) {
	tests := []struct {
		in      string
		want    Origin
		wantErr bool
	}{
		{in: "https://example.com", want: Origin{Scheme: "https", Host: "example.com"}},
		{in: "https://example.com:8443", want: Origin{Scheme: "https", Host: "example.com", Port: "8443"}},
		{in: "http://localhost:5173", want: Origin{Scheme: "http", Host: "localhost", Port: "5173"}},
		{in: "https://[::1]:8443", want: Origin{Scheme: "https", Host: "[::1]", Port: "8443"}},
		{in: "https://[::1]", want: Origin{Scheme: "https", Host: "[::1]"}},
		{in: "https://*.example.com", want: Origin{Scheme: "https", Host: "*.example.com"}},
		// Port boundaries: 1 and 65535 are the smallest/largest ports a browser
		// can send and must be accepted, not just the values one past them.
		{in: "https://example.com:1", want: Origin{Scheme: "https", Host: "example.com", Port: "1"}},
		{in: "https://example.com:65535", want: Origin{Scheme: "https", Host: "example.com", Port: "65535"}},

		// A browser serializes an explicit default port away, so an allow-list
		// entry spelled either way must parse to the same Origin.
		{in: "https://example.com:443", want: Origin{Scheme: "https", Host: "example.com"}},
		{in: "http://example.com:80", want: Origin{Scheme: "http", Host: "example.com"}},
		// The OTHER scheme's default port is not normalized away — it is a
		// genuinely different, non-default port for this scheme.
		{in: "https://example.com:80", want: Origin{Scheme: "https", Host: "example.com", Port: "80"}},
		{in: "http://example.com:443", want: Origin{Scheme: "http", Host: "example.com", Port: "443"}},
		// A scheme other than http/https has no default port to normalize away.
		{in: "ftp://example.com:21", want: Origin{Scheme: "ftp", Host: "example.com", Port: "21"}},

		{in: "null", wantErr: true},
		{in: "example.com", wantErr: true},    // no scheme
		{in: "://example.com", wantErr: true}, // empty scheme
		{in: "https://example.com/path", wantErr: true},
		{in: "https://example.com?x=1", wantErr: true},  // query: a browser never sends this in Origin
		{in: "https://example.com#frag", wantErr: true}, // fragment: same reason
		{in: "https://", wantErr: true},                 // no host
		{in: "https://example.com:", wantErr: true},     // empty port
		{in: "https://example.com:0", wantErr: true},    // port out of range
		{in: "https://example.com:70000", wantErr: true},
		{in: "https://example.com:abc", wantErr: true},
		{in: "https://[::1]x", wantErr: true}, // junk after the IPv6 bracket, not ":port"
		{in: "https://[::1", wantErr: true},   // unbalanced IPv6 bracket
	}
	for _, tc := range tests {
		got, err := ParseOrigin(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseOrigin(%q) = %+v, want error", tc.in, got)
			}
			if got != (Origin{}) {
				t.Errorf("ParseOrigin(%q) = %+v, want zero value on error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseOrigin(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseOrigin(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestOriginUnbalancedIPv6BracketIsRejected(t *testing.T) {
	// An unbalanced "[" must be rejected, not passed through as a host: it can
	// never match a real browser Origin header, so letting it into
	// corsPatterns would silently and permanently disable CORS for that entry.
	got, err := ParseOrigin("https://[::1")
	if err == nil {
		t.Fatalf("ParseOrigin(%q) = %+v, want error", "https://[::1", got)
	}
	if got != (Origin{}) {
		t.Errorf("ParseOrigin(%q) = %+v, want zero value on error", "https://[::1", got)
	}
}
