package httpapi

import (
	"net/http"
	"strconv"
)

// handleSpeciesSearch answers the index-own name search.
//
// A limit key that is absent altogether is nil ("unset") and lets the use
// case apply its default; a limit key that is present — including an
// explicitly empty "?limit=" — but fails to parse is rejected rather than
// treated as absent, because a typo'd limit silently answering a different
// question is the failure mode this route would otherwise have. Presence is
// checked with Query().Has, since Query().Get alone cannot tell "missing"
// from "present but empty" apart. Parsability is the only check made here:
// it is genuinely HTTP's concern, the only place the raw string exists.
// Whether q is empty and whether limit is in range — an explicit 0 included
// — are rules of the query itself, not of the transport: the use case
// enforces those, for every future caller, not just this one handler.
func (s *Server) handleSpeciesSearch(w http.ResponseWriter, r *http.Request) {
	var limit *int
	if r.URL.Query().Has("limit") {
		parsed, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil {
			s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
				"limit must be a whole number")
			return
		}
		limit = &parsed
	}

	hits, err := s.deps.Query.SearchSpecies(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, hits)
}
