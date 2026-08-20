package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// maxBatchBodyBytes bounds the batch request body. A read API must not let one
// request pull an unbounded amount into memory.
const maxBatchBodyBytes = 1 << 20

// maxBatchConceptIDs bounds the length of the concept_ids array, which the byte
// cap does not: 1 MiB of short ids is tens of thousands of entries, and each
// distinct one costs a handful of index queries. 300 is well above any realistic
// field record (an excursion plot list is tens of species, not hundreds) while
// keeping the worst case bounded. It is mirrored as `maxItems` in the OpenAPI
// spec, which bounds array length, so both agree by construction.
const maxBatchConceptIDs = 300

// handleSpeciesHabitatTypes answers GET /v1/species/{conceptId}/habitat-types —
// the excursion app's main question, and autark: a concept ID needs no upstream.
func (s *Server) handleSpeciesHabitatTypes(w http.ResponseWriter, r *http.Request) {
	conceptID := strings.TrimSpace(mux.Vars(r)["conceptId"])
	if conceptID == "" {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "conceptId is empty")
		return
	}
	filter, ferr := areaFilter(r)
	if ferr != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, ferr.Error())
		return
	}
	types, err := s.deps.Query.SpeciesHabitatTypes(r.Context(), conceptID, language(r), filter)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, types)
}

// batchRequest is the body of POST /v1/species/habitat-types: concept ids the
// caller already resolved (against hostus, its own cache, whatever) — situs
// resolves no verbatim name at runtime. DisallowUnknownFields is what makes the
// removed `names` field a 400 rather than an empty answer.
type batchRequest struct {
	ConceptIDs []string `json:"concept_ids"`
}

func (s *Server) handleSpeciesBatch(w http.ResponseWriter, r *http.Request) {
	filter, ferr := areaFilter(r)
	if ferr != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, ferr.Error())
		return
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBatchBodyBytes))
	dec.DisallowUnknownFields()
	var req batchRequest
	if err := dec.Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "request body must be {\"concept_ids\":[...]}")
		return
	}
	// Without this, a body of two concatenated objects decodes the first and
	// silently discards the rest — the caller would believe it sent both.
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "request body must hold exactly one JSON object")
		return
	}

	// The bound is on the raw array length, because that is what `maxItems: 300`
	// in the spec means — a validating gateway or generated client must reach
	// the same verdict as this handler, and none of them reads prose.
	if len(req.ConceptIDs) > maxBatchConceptIDs {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			fmt.Sprintf("concept_ids holds %d entries, at most %d are accepted",
				len(req.ConceptIDs), maxBatchConceptIDs))
		return
	}

	// Every non-blank id in input order, duplicates included: the answer carries
	// one entry per input so response[i] pairs with concept_ids[i]. Deduplicating
	// the index work is the use case's job, not the adapter's.
	asked := make([]string, 0, len(req.ConceptIDs))
	for _, id := range req.ConceptIDs {
		if id = strings.TrimSpace(id); id != "" {
			asked = append(asked, id)
		}
	}
	if len(asked) == 0 {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			"concept_ids must hold at least one non-empty concept id")
		return
	}

	resolutions, err := s.deps.Query.SpeciesSetHabitatTypes(r.Context(), asked, language(r), filter)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, resolutions)
}
