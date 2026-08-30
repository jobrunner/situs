package domain

import (
	"fmt"
	"strings"
)

// TraitDim identifies one measured dimension within a trait vocabulary
// (e.g. "M" for EIVE's moisture axis, "disturbance_severity" for Midolo's
// overall disturbance intensity). Not a closed enum: each vocabulary
// defines its own set of dimensions, situs keeps no cross-vocabulary
// dimension registry.
type TraitDim string

// ParseTraitDim validates only that s is not empty — the spelling is a
// per-vocabulary convention, not a situs-wide register.
func ParseTraitDim(s string) (TraitDim, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("trait dimension is empty")
	}
	return TraitDim(s), nil
}

// TraitValue is one indicator value for one concept in one trait
// vocabulary. NicheWidth/NSystems are nil when the vocabulary does not
// provide them (Tichý/Midolo never do; EIVE always does) — never coerced to
// 0.0/0.
type TraitValue struct {
	Vocab        string
	VocabVersion string
	Dim          TraitDim
	Value        float64
	NicheWidth   *float64
	NSystems     *int
}

// TraitSet groups every TraitValue one vocabulary contributes for one
// concept — never merged across vocabularies (M in EIVE and M in Tichý are
// different measurements on different scales).
type TraitSet struct {
	Vocab        string
	VocabVersion string
	Values       []TraitValue
}
