package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jobrunner/situs/internal/ports/input"
)

// handleSpeciesSearch answers the index-own name search.
//
// An absent limit is 0 ("unset") and lets the use case apply its default; an
// unparsable one is rejected rather than treated as absent, because a typo'd
// limit silently answering a different question is the failure mode this
// route would otherwise have. q emptiness and the limit bound are checked
// here too, not only in the use case — the same convention as the role and
// only_in_area checks on the sibling routes: a double whose answers are
// query-shaped (like the test doubles this adapter is tested against) cannot
// stand in for a validation the use case alone performs.
func (s *Server) handleSpeciesSearch(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("q")) == "" {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "q must not be empty")
		return
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
				"limit must be a whole number")
			return
		}
		if parsed < 0 || parsed > input.MaxSearchLimit {
			s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
				"limit must be between 1 and the maximum")
			return
		}
		limit = parsed
	}

	hits, err := s.deps.Query.SearchSpecies(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, hits)
}
