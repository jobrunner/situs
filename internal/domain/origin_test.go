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

		{in: "null", wantErr: true},
		{in: "example.com", wantErr: true},    // no scheme
		{in: "://example.com", wantErr: true}, // empty scheme
		{in: "https://example.com/path", wantErr: true},
		{in: "https://", wantErr: true},              // no host
		{in: "https://example.com:", wantErr: true},  // empty port
		{in: "https://example.com:0", wantErr: true}, // port out of range
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
