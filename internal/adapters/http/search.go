package httpapi

import (
	"net/http"
	"strconv"
)

// handleSpeciesSearch answers the index-own name search.
//
// An absent limit is 0 ("unset") and lets the use case apply its default; an
// unparsable one is rejected rather than treated as absent, because a typo'd
// limit silently answering a different question is the failure mode this
// route would otherwise have. Parsability is the only check made here: it is
// genuinely HTTP's concern, the only place the raw string exists. Whether q
// is empty and whether limit is in range are rules of the query itself, not
// of the transport — the use case enforces those, for every future caller,
// not just this one handler.
func (s *Server) handleSpeciesSearch(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
				"limit must be a whole number")
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
