package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/jobrunner/situs/internal/ports/input"
)

// maxBatchBodyBytes bounds the batch request body. A read API must not let one
// request pull an unbounded amount into memory.
const maxBatchBodyBytes = 1 << 20

// maxBatchNames bounds the length of the names array, which the byte cap does
// not: 1 MiB of short names is roughly 50 000 entries, and every 50 of them cost
// one hostus round trip measured at up to 16.3s — one request could occupy a
// connection for hours and hammer hostus. 300 is well above any realistic field
// record (an excursion plot list is tens of species, not hundreds) while capping
// the worst case at six round trips. It is mirrored as `maxItems` in the
// OpenAPI spec, which bounds array length, so both agree by construction.
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

	// The bound is on the raw array length, because that is what `maxItems: 300`
	// in the spec means — a validating gateway or generated client must reach
	// the same verdict as this handler, and none of them reads prose.
	if len(req.Names) > maxBatchNames {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			fmt.Sprintf("names holds %d entries, at most %d are accepted", len(req.Names), maxBatchNames))
		return
	}

	// Two sequences from one input: asked keeps every non-blank name in input
	// order, duplicates included, because the answer carries one entry per input
	// name so response[i] pairs with input[i]. distinct is what goes upstream —
	// deduplication is the caller's job per output.NameResolver, and it is what
	// bounds the hostus round trips.
	asked := make([]string, 0, len(req.Names))
	distinct := make([]string, 0, len(req.Names))
	seen := make(map[string]struct{}, len(req.Names))
	for _, n := range req.Names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		asked = append(asked, n)
		if _, dup := seen[n]; !dup {
			seen[n] = struct{}{}
			distinct = append(distinct, n)
		}
	}
	if len(asked) == 0 {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "names must hold at least one non-empty name")
		return
	}

	resolutions, err := s.deps.Names.SpeciesHabitatTypesByName(r.Context(), distinct, language(r))
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, fanOutByInputName(asked, resolutions))
}

// fanOutByInputName expands the per-distinct-name resolutions back over the
// input sequence, so a duplicated name simply yields the same resolution twice
// and a client may pair response[i] with input[i].
func fanOutByInputName(asked []string, resolutions []input.NameResolution) []input.NameResolution {
	byName := make(map[string]input.NameResolution, len(resolutions))
	for _, res := range resolutions {
		byName[res.Verbatim] = res
	}

	out := make([]input.NameResolution, 0, len(asked))
	for _, n := range asked {
		res, ok := byName[n]
		if !ok {
			// A resolution the use case did not answer about must still be
			// reported back — an input name is never silently dropped.
			res = input.NameResolution{Verbatim: n, HabitatTypes: []input.HabitatTypeRole{}}
		}
		out = append(out, res)
	}
	return out
}
