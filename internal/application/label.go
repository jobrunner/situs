package application

import (
	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// The German label overlay lives here rather than in query.go: deciding which of
// several localization rows to serve, and which of them may be paired with which,
// is its own concern with its own rules — query.go orchestrates endpoints.

// preferredLabel picks the label to serve and reports its provenance, ordered
// official > curated > derived > situs: the most authoritative wording wins, and
// the answer never depends on row order. Repository.Localization does order its
// rows, but this holds regardless of whether an implementation does. curated
// outranks derived because a human deliberately intervened there; situs is last
// because nothing outside situs vouches for it.
//
// The vernacular is attached only from the same provenance AND the same source as
// the winning name. Provenance alone is not enough: two sources can share a
// provenance (an official EU wording and an official national one), and pairing
// across them would attach a term from one source to a name from another while
// the object reports only the name's source — the provenance field would stay
// true and the source field would lie about half the object.
func preferredLabel(labels []domain.Localization) *input.GermanLabel {
	names, vernaculars := labelsByOrigin(labels)
	for _, provenance := range []string{
		provenanceOfficial, provenanceCurated, provenanceDerived, provenanceSitus,
	} {
		n, ok := names[provenance]
		if !ok {
			continue
		}
		return &input.GermanLabel{
			Value:      n.Value,
			Vernacular: vernaculars[labelOrigin{n.Provenance, n.Source}].Value,
			Provenance: n.Provenance,
			Source:     n.Source,
		}
	}
	return nil
}

// preferredDescription picks the German description to serve, by the same
// ranking as preferredLabel. It carries no vernacular: a description has no
// short established form to pair with, only a wording and its origin.
func preferredDescription(labels []domain.Localization) *input.GermanLabel {
	byProvenance := map[string]domain.Localization{}
	for _, l := range labels {
		if l.Field != descriptionField {
			continue
		}
		if _, seen := byProvenance[l.Provenance]; !seen {
			byProvenance[l.Provenance] = l
		}
	}
	for _, provenance := range []string{
		provenanceOfficial, provenanceCurated, provenanceDerived, provenanceSitus,
	} {
		if l, ok := byProvenance[provenance]; ok {
			return &input.GermanLabel{Value: l.Value, Provenance: l.Provenance, Source: l.Source}
		}
	}
	return nil
}

// labelOrigin is what identifies where a label came from. Both halves matter:
// see the pairing rule on preferredLabel.
type labelOrigin struct {
	provenance string
	source     string
}

// labelsByOrigin buckets the rows of one entity: names by provenance (the
// ranking key) and vernaculars by their full origin (the pairing key). The first
// row of each bucket wins, which is why the repository promises a total order.
func labelsByOrigin(labels []domain.Localization) (
	map[string]domain.Localization, map[labelOrigin]domain.Localization,
) {
	names := map[string]domain.Localization{}
	vernaculars := map[labelOrigin]domain.Localization{}
	for _, l := range labels {
		switch l.Field {
		case nameField:
			if _, seen := names[l.Provenance]; !seen {
				names[l.Provenance] = l
			}
		case vernacularField:
			key := labelOrigin{l.Provenance, l.Source}
			if _, seen := vernaculars[key]; !seen {
				vernaculars[key] = l
			}
		}
	}
	return names, vernaculars
}
