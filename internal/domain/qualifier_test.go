package domain

import "testing"

func TestParseQualifier(t *testing.T) {
	tests := []struct {
		in      string
		want    Qualifier
		wantErr bool
	}{
		{in: "=", want: QualifierSame},
		{in: "<", want: QualifierNarrower},
		{in: ">", want: QualifierBroader},
		{in: "#", want: QualifierPartial},
		{in: " = ", want: QualifierSame},
		{in: "", wantErr: true},
		{in: "==", wantErr: true},
		{in: "~", wantErr: true},
	}
	for _, tc := range tests {
		got, err := ParseQualifier(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseQualifier(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseQualifier(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseQualifier(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Only '=' means full correspondence — the derived German label rule depends
// on exactly this distinction, so it is pinned here.
func TestQualifierIsSame(t *testing.T) {
	if !QualifierSame.IsSame() {
		t.Error("QualifierSame.IsSame() = false, want true")
	}
	for _, q := range []Qualifier{QualifierNarrower, QualifierBroader, QualifierPartial} {
		if q.IsSame() {
			t.Errorf("%q.IsSame() = true, want false", q)
		}
	}
}
