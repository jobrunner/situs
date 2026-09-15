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

func TestOriginUnbalancedIPv6BracketIsTreatedAsHost(t *testing.T) {
	// An unbalanced "[" is not rejected by splitHostPort itself (it falls back to
	// treating the whole remainder as the host); it still parses successfully
	// since nothing else about it is malformed.
	got, err := ParseOrigin("https://[::1")
	if err != nil {
		t.Fatalf("ParseOrigin(%q): unexpected error %v", "https://[::1", err)
	}
	want := Origin{Scheme: "https", Host: "[::1"}
	if got != want {
		t.Errorf("ParseOrigin(%q) = %+v, want %+v", "https://[::1", got, want)
	}
}
