package httpapi

import (
	"net/http"
)

// handleSpeciesTraitSummary answers the indicator-value analysis.
//
// Body handling is decodeConceptIDs, shared with handleSpeciesBatch: same
// shape, same limits, same refusal to silently swallow a second JSON object
// — because a caller that learned one of the two routes must not be
// surprised by the other.
func (s *Server) handleSpeciesTraitSummary(w http.ResponseWriter, r *http.Request) {
	asked, ok := s.decodeConceptIDs(w, r)
	if !ok {
		return
	}

	summary, err := s.deps.Query.SpeciesTraitSummary(r.Context(), asked)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, summary)
}
