package domain

import "testing"

func TestParseTypologyID(t *testing.T) {
	tests := []struct {
		in          string
		wantScheme  string
		wantVersion string
		wantErr     bool
	}{
		{in: "eunis@2021", wantScheme: "eunis", wantVersion: "2021"},
		{in: "eunis@2012", wantScheme: "eunis", wantVersion: "2012"},
		{in: "annex1", wantScheme: "annex1", wantVersion: ""},
		{in: "  eunis@2021  ", wantScheme: "eunis", wantVersion: "2021"},
		{in: "", wantErr: true},
		{in: "@2021", wantErr: true},
		{in: "eunis@", wantErr: true},
		{in: "eunis@2021@x", wantErr: true},
	}
	for _, tc := range tests {
		got, err := ParseTypologyID(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseTypologyID(%q) = %q, want error", tc.in, got)
			}
			if got != TypologyID("") {
				t.Errorf("ParseTypologyID(%q) = %q, want zero value on error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTypologyID(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got.Scheme() != tc.wantScheme || got.Version() != tc.wantVersion {
			t.Errorf("ParseTypologyID(%q) = scheme %q version %q, want %q/%q",
				tc.in, got.Scheme(), got.Version(), tc.wantScheme, tc.wantVersion)
		}
	}
}

func TestHabitatTypeKeyString(t *testing.T) {
	k := HabitatTypeKey{Typology: TypologyID("eunis@2021"), Code: "R22"}
	if got, want := k.String(), "eunis@2021:R22"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
