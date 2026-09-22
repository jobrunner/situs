package domain

import (
	"slices"
	"testing"
)

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

func TestSchemaKonstantenWerte(t *testing.T) {
	// The values sit in a schema CHECK, in query strings and in published
	// JSON: a rename must not pass silently.
	for got, want := range map[string]string{
		SchemeWGSRPDL3:      "wgsrpd_l3",
		SchemeEVCTerritory:  "evc_territory",
		OccurrenceVerified:  "verified",
		OccurrenceUncertain: "uncertain",
	} {
		if got != want {
			t.Errorf("Konstante = %q, erwartet %q", got, want)
		}
	}
}

func TestIsKnownAreaSchemeKenntBeideUndSonstNichts(t *testing.T) {
	for _, scheme := range []string{SchemeWGSRPDL3, SchemeEVCTerritory} {
		if !IsKnownAreaScheme(scheme) {
			t.Errorf("IsKnownAreaScheme(%q) = false", scheme)
		}
	}
	for _, scheme := range []string{"", "wgsrpd-l3", "evc-territory", "WGSRPD_L3", "iso3166"} {
		if IsKnownAreaScheme(scheme) {
			t.Errorf("IsKnownAreaScheme(%q) = true, erwartet false", scheme)
		}
	}
}

func TestKnownAreaSchemesIstSortiertUndVollstaendig(t *testing.T) {
	got := KnownAreaSchemes()
	want := []string{"evc_territory", "wgsrpd_l3"}
	if !slices.Equal(got, want) {
		t.Errorf("KnownAreaSchemes() = %v, erwartet %v", got, want)
	}
	// The caller puts this list into an INVALID_QUERY message; a caller
	// mutating it must not change what the next request is told.
	got[0] = "geaendert"
	if KnownAreaSchemes()[0] != "evc_territory" {
		t.Error("KnownAreaSchemes() gibt den internen Slice heraus")
	}
}
