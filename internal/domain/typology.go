package domain

import (
	"fmt"
	"strings"
)

// TypologyID identifies a habitat classification system in a given fassung,
// e.g. "eunis@2021" or "annex1" (which carries no version).
type TypologyID string

// ParseTypologyID validates "<scheme>" or "<scheme>@<version>".
func ParseTypologyID(s string) (TypologyID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("typology id is empty")
	}
	scheme, version, hasAt := strings.Cut(s, "@")
	if scheme == "" {
		return "", fmt.Errorf("typology id %q has no scheme", s)
	}
	if hasAt && version == "" {
		return "", fmt.Errorf("typology id %q has an empty version", s)
	}
	if strings.Contains(version, "@") {
		return "", fmt.Errorf("typology id %q has more than one %q", s, "@")
	}
	return TypologyID(s), nil
}

func (t TypologyID) Scheme() string {
	scheme, _, _ := strings.Cut(string(t), "@")
	return scheme
}

func (t TypologyID) Version() string {
	_, version, _ := strings.Cut(string(t), "@")
	return version
}

// HabitatTypeKey identifies an abstract habitat type. A type is never
// identified by its code alone — the same code means different things in
// different typologies.
type HabitatTypeKey struct {
	Typology TypologyID
	Code     string
}

func (k HabitatTypeKey) String() string {
	return string(k.Typology) + ":" + k.Code
}
