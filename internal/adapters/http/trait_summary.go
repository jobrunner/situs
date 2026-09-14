package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// handleSpeciesTraitSummary answers the indicator-value analysis.
//
// Body handling mirrors handleSpeciesBatch exactly — same shape, same
// limits, same refusal to silently swallow a second JSON object — because a
// caller that learned one of the two routes must not be surprised by the
// other.
func (s *Server) handleSpeciesTraitSummary(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBatchBodyBytes))
	dec.DisallowUnknownFields()
	var req batchRequest
	if err := dec.Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			"request body must be {\"concept_ids\":[...]}")
		return
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			"request body must hold exactly one JSON object")
		return
	}
	if len(req.ConceptIDs) > maxBatchConceptIDs {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			fmt.Sprintf("concept_ids holds %d entries, at most %d are accepted",
				len(req.ConceptIDs), maxBatchConceptIDs))
		return
	}

	asked := make([]string, 0, len(req.ConceptIDs))
	for i, id := range req.ConceptIDs {
		if id = strings.TrimSpace(id); id == "" {
			s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
				fmt.Sprintf("concept_ids[%d] is empty; every entry must be a concept id", i))
			return
		}
		asked = append(asked, id)
	}

	summary, err := s.deps.Query.SpeciesTraitSummary(r.Context(), asked)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, summary)
}
