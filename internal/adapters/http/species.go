package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// maxBatchBodyBytes bounds the batch request body. A read API must not let one
// request pull an unbounded amount into memory.
const maxBatchBodyBytes = 1 << 20

// handleSpeciesHabitatTypes answers GET /v1/species/{conceptId}/habitat-types —
// the excursion app's main question, and autark: a concept ID needs no upstream.
func (s *Server) handleSpeciesHabitatTypes(w http.ResponseWriter, r *http.Request) {
	conceptID := strings.TrimSpace(mux.Vars(r)["conceptId"])
	if conceptID == "" {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "conceptId is empty")
		return
	}
	types, err := s.deps.Query.SpeciesHabitatTypes(r.Context(), conceptID, language(r))
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, types)
}

// batchRequest is the body of POST /v1/species/habitat-types: verbatim names as
// recorded in the field, resolved through hostus before the index is queried.
type batchRequest struct {
	Names []string `json:"names"`
}

func (s *Server) handleSpeciesBatch(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBatchBodyBytes))
	dec.DisallowUnknownFields()
	var req batchRequest
	if err := dec.Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "request body must be {\"names\":[...]}")
		return
	}

	names := make([]string, 0, len(req.Names))
	for _, n := range req.Names {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "names must hold at least one non-empty name")
		return
	}

	resolutions, err := s.deps.Names.SpeciesHabitatTypesByName(r.Context(), names, language(r))
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, resolutions)
}
