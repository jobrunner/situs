package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// maxBatchBodyBytes bounds the batch request body. A read API must not let one
// request pull an unbounded amount into memory.
const maxBatchBodyBytes = 1 << 20

// maxBatchNames bounds the item count, which the byte cap does not: 1 MiB of
// short names is roughly 50 000 entries, and every 50 of them cost one hostus
// round trip measured at up to 16.3s — one request could occupy a connection
// for hours and hammer hostus. 300 is well above any realistic field record
// (an excursion plot list is tens of species, not hundreds) while capping the
// worst case at six round trips.
const maxBatchNames = 300

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

	// Dedupe here, preserving the caller's input order: output.NameResolver's
	// contract puts deduplication on the caller, and a field recording easily
	// repeats a name. One resolution entry per distinct input name.
	names := make([]string, 0, len(req.Names))
	seen := make(map[string]struct{}, len(req.Names))
	for _, n := range req.Names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	if len(names) == 0 {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "names must hold at least one non-empty name")
		return
	}
	if len(names) > maxBatchNames {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			fmt.Sprintf("names holds %d distinct entries, at most %d are accepted", len(names), maxBatchNames))
		return
	}

	resolutions, err := s.deps.Names.SpeciesHabitatTypesByName(r.Context(), names, language(r))
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, resolutions)
}
