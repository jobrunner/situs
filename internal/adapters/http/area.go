package httpapi

import "net/http"

// handleAreas answers GET /v1/areas: the distribution areas the index has
// data for, with their WGSRPD names, sorted by code. It is the discovery
// entry point for ?area= — without it a client has to know the WGSRPD code
// table by heart, and still could not tell which codes THIS index can answer.
//
// Deliberately only the areas with data: an area nobody occurs in would be a
// filter that can only ever answer empty.
func (s *Server) handleAreas(w http.ResponseWriter, r *http.Request) {
	areas, err := s.deps.Query.Areas(r.Context())
	if err != nil {
		s.logger.ErrorContext(r.Context(), "listing areas", "error", err)
		s.writeError(w, http.StatusInternalServerError, CodeInternalError, "internal error")
		return
	}
	s.writeJSON(w, http.StatusOK, areas)
}
