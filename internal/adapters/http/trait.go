package httpapi

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// handleSpeciesTraits answers GET /v1/species/{conceptId}/traits[?vocab=].
// Autark like every other species route: a concept id needs no upstream. A
// concept without trait data answers 200 with an empty array, never 404 —
// "no data" is a normal answer here, the same stance situs already takes
// for habitat assignments.
func (s *Server) handleSpeciesTraits(w http.ResponseWriter, r *http.Request) {
	conceptID := strings.TrimSpace(mux.Vars(r)["conceptId"])
	if conceptID == "" {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "conceptId is empty")
		return
	}
	vocab := strings.TrimSpace(r.URL.Query().Get("vocab"))
	sets, err := s.deps.Query.Traits(r.Context(), conceptID, vocab)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, sets)
}
