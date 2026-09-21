package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
)

// handleAreas answers GET /v1/areas: the distribution areas the index has
// data for, with their names, sorted by code. It is the discovery entry point
// for ?area= — without it a client has to know the code table by heart, and
// still could not tell which codes THIS index can answer.
//
// Deliberately only the areas with data: an area nobody occurs in would be a
// filter that can only ever answer empty.
//
// ?scheme= defaults to wgsrpd_l3, which is what this route answered before
// the second scheme existed — so every request made until now keeps its
// answer. One scheme per request, never both merged: a flat list of
// "albania" next to "GER" would be two vocabularies in one selection.
func (s *Server) handleAreas(w http.ResponseWriter, r *http.Request) {
	scheme := strings.TrimSpace(r.URL.Query().Get("scheme"))
	if scheme == "" {
		scheme = domain.SchemeWGSRPDL3
	}
	if !domain.IsKnownAreaScheme(scheme) {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			fmt.Sprintf("scheme %q is unknown; allowed: %s",
				scheme, strings.Join(domain.KnownAreaSchemes(), ", ")))
		return
	}

	areas, err := s.deps.Query.Areas(r.Context(), scheme)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "listing areas", "error", err, "scheme", scheme)
		s.writeError(w, http.StatusInternalServerError, CodeInternalError, "internal error")
		return
	}
	s.writeJSON(w, http.StatusOK, areas)
}
