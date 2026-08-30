package domain

import "testing"

func TestParseTraitDim_RejectsEmpty(t *testing.T) {
	if _, err := ParseTraitDim(""); err == nil {
		t.Fatal("ParseTraitDim(\"\") = nil error, want error")
	}
	if _, err := ParseTraitDim("   "); err == nil {
		t.Fatal("ParseTraitDim(\"   \") = nil error, want error")
	}
}

func TestParseTraitDim_TrimsAndAccepts(t *testing.T) {
	dim, err := ParseTraitDim("  M  ")
	if err != nil {
		t.Fatalf("ParseTraitDim: %v", err)
	}
	if dim != TraitDim("M") {
		t.Errorf("dim = %q, want %q", dim, "M")
	}
}
