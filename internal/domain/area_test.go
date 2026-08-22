package domain

import "testing"

func TestAreaString(t *testing.T) {
	a := Area{Scheme: SchemeWGSRPDL3, Code: "GER"}
	if got, want := a.String(), "wgsrpd_l3:GER"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// An area without a scheme is not addressable — the same code means different
// places in different schemes, exactly like a habitat type code.
func TestAreaIsComplete(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    Area
		want bool
	}{
		{name: "both set", a: Area{Scheme: SchemeWGSRPDL3, Code: "GER"}, want: true},
		{name: "no code", a: Area{Scheme: SchemeWGSRPDL3}, want: false},
		{name: "no scheme", a: Area{Code: "GER"}, want: false},
		{name: "empty", a: Area{}, want: false},
	} {
		if got := tc.a.IsComplete(); got != tc.want {
			t.Errorf("%s: IsComplete() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
