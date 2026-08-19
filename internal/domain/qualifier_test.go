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
		{in: "≈", want: QualifierApproximate},
		{in: " ≈ ", want: QualifierApproximate},
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
			if got != Qualifier("") {
				t.Errorf("ParseQualifier(%q) = %q, want zero value on error", tc.in, got)
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

// A crosswalk row is stored once and answered from both ends, so the inverse
// must flip the direction and only the direction.
func TestQualifierInverse(t *testing.T) {
	for in, want := range map[Qualifier]Qualifier{
		QualifierNarrower: QualifierBroader,
		QualifierBroader:  QualifierNarrower,
		QualifierSame:     QualifierSame,
		QualifierPartial:  QualifierPartial,
		// Symmetric by nature, and deliberately so: approximate from one end is
		// approximate from the other.
		QualifierApproximate: QualifierApproximate,
		// An unknown symbol is returned unchanged rather than guessed.
		Qualifier("?"): Qualifier("?"),
	} {
		if got := in.Inverse(); got != want {
			t.Errorf("%q.Inverse() = %q, want %q", in, got, want)
		}
		if got := in.Inverse().Inverse(); got != in {
			t.Errorf("%q.Inverse().Inverse() = %q, want the original", in, got)
		}
	}
}

// Only '=' means full correspondence — the derived German label rule depends
// on exactly this distinction, so it is pinned here.
func TestQualifierIsSame(t *testing.T) {
	if !QualifierSame.IsSame() {
		t.Error("QualifierSame.IsSame() = false, want true")
	}
	for _, q := range []Qualifier{QualifierNarrower, QualifierBroader, QualifierPartial, QualifierApproximate} {
		if q.IsSame() {
			t.Errorf("%q.IsSame() = true, want false", q)
		}
	}
}
