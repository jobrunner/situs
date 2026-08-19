package domain

import (
	"fmt"
	"strings"
)

// Qualifier is the coverage of a crosswalk correspondence, verbatim from the
// EUNIS sources.
type Qualifier string

const (
	QualifierSame     Qualifier = "=" // full correspondence
	QualifierNarrower Qualifier = "<"
	QualifierBroader  Qualifier = ">"
	QualifierPartial  Qualifier = "#"
	// QualifierApproximate is admitted rather than dropped: the EEA Annex I
	// sheet uses it for real conservation correspondences (R1S→6130, U28→8130),
	// and an ecologist asking which EUNIS types match an LRT must not get a
	// silently short answer. It is safe for the derived-German-label rule
	// because that rule is gated on IsSame, which is false here.
	QualifierApproximate Qualifier = "≈"
)

func ParseQualifier(s string) (Qualifier, error) {
	switch q := Qualifier(strings.TrimSpace(s)); q {
	case QualifierSame, QualifierNarrower, QualifierBroader, QualifierPartial, QualifierApproximate:
		return q, nil
	default:
		return "", fmt.Errorf("unknown crosswalk qualifier %q", s)
	}
}

// IsSame reports full correspondence. Only these may seed a derived German
// label for a EUNIS type (see the spec).
func (q Qualifier) IsSame() bool { return q == QualifierSame }

// Inverse is the qualifier seen from the other end of the correspondence: a
// crosswalk row is stored once but answerable from both types, and "A is
// narrower than B" must read as "B is broader than A" when B is the one asked
// about. '=', '#' and '≈' are symmetric — an approximate correspondence is
// approximate from either end — so they are returned unchanged, as is anything
// unknown, rather than guessed.
func (q Qualifier) Inverse() Qualifier {
	switch q {
	case QualifierNarrower:
		return QualifierBroader
	case QualifierBroader:
		return QualifierNarrower
	default:
		return q
	}
}
