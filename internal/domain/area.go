package domain

import "slices"

// SchemeWGSRPDL3 is the only area scheme situs stores today: WGSRPD level 3
// ("botanical countries"), which is what hostus reports per concept.
const SchemeWGSRPDL3 = "wgsrpd_l3"

// SchemeEVCTerritory is the second area scheme: the 136 territory columns of
// the EVC alliance distribution table. They are NOT WGSRPD areas — some are
// states ("Albania"), some biogeographical parts of one ("Austria Alps",
// "France Mediterranean"), and most have a coastal counterpart
// ("Albania_coast").
//
// There is no mapping between the two schemes and there is not meant to be
// one, the same stance CLAUDE.md already records for ISO<->WGSRPD. Species
// distribution is filtered with WGSRPD codes, syntaxon distribution with
// territory codes, and no answer claims a conversion nobody has checked.
const SchemeEVCTerritory = "evc_territory"

// The occurrence states the source distinguishes: "1" and "U" in its cells.
// Absence is the ABSENCE of a row and therefore has no constant — giving it
// one would invite writing it.
const (
	OccurrenceVerified  = "verified"
	OccurrenceUncertain = "uncertain"
)

// knownAreaSchemes is checked against, rather than one scheme being hardcoded
// at each site: the ingest used to reject anything but wgsrpd_l3, which was
// right while there was one scheme and would have silently dropped all 136
// territories once there were two. Checking the SET keeps a typo failing.
var knownAreaSchemes = []string{SchemeEVCTerritory, SchemeWGSRPDL3}

// KnownAreaSchemes returns the area schemes situs stores, sorted. Callers put
// this into an INVALID_QUERY message, so it hands out a copy.
func KnownAreaSchemes() []string { return slices.Clone(knownAreaSchemes) }

// IsKnownAreaScheme reports whether scheme is one situs stores. Exact match,
// no case folding: the scheme id is an identifier, not free text.
func IsKnownAreaScheme(scheme string) bool { return slices.Contains(knownAreaSchemes, scheme) }

// SyntaxonDistribution is what the source says about one syntaxon in one area
// scheme.
//
// Covered is the field that keeps four states apart, and it is the whole
// reason this is a struct rather than two slices: with Covered false NOTHING
// is known about this syntaxon's occurrence, which is strictly different from
// "occurs nowhere" (Covered true, both slices empty). Measured: 212 of the
// index's 1326 alliances are the first case — every bryophyte, lichen and
// algal alliance among them — and reporting them as absences would be an
// invented claim about 136 territories each.
type SyntaxonDistribution struct {
	Scheme    string
	Covered   bool
	Verified  []string
	Uncertain []string
}

// Area is a distribution area. Scheme and Code together identify it — a bare
// code is ambiguous across schemes, the same way a habitat type code is
// ambiguous across typologies.
type Area struct {
	Scheme string
	Code   string
}

func (a Area) String() string { return a.Scheme + ":" + a.Code }

// IsComplete reports whether both halves are present. An incomplete area must
// never reach the index: it would silently match nothing.
func (a Area) IsComplete() bool { return a.Scheme != "" && a.Code != "" }

// NamedArea is an area plus its English name from the WGSRPD tables. The name
// is an overlay on the code, never its identity: the index answers in codes,
// and an area whose name was never ingested keeps an empty NameEN rather than
// borrowing its code as a stand-in name.
type NamedArea struct {
	Area
	NameEN string
}
