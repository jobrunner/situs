package httpapi

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gorilla/mux"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// defaultSyntaxonRank is what GET /v1/syntaxa answers without ?rank=: the
// formations, the roots of the hierarchy.
//
// The default lives HERE and not in the port: the port knows no special case
// for an empty rank, and a default that dumped all 1882 rows would be an answer
// nobody asked for. Keeping the parameter checks in the handler is also what
// lets a further filter (subproject C adds ?area= and ?include=) be added
// without rebuilding the route.
const defaultSyntaxonRank = domain.SyntaxonRankFormation

// handleSyntaxa answers GET /v1/syntaxa?rank=&life_form_group= — the entry
// point of the hierarchy, and the only route a client needs to know by heart.
func (s *Server) handleSyntaxa(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rank := strings.TrimSpace(q.Get("rank"))
	if rank == "" {
		rank = defaultSyntaxonRank
	}
	group := strings.TrimSpace(q.Get("life_form_group"))
	if !validLifeFormGroup(group) {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			"life_form_group must be one of phanerogam, bryophyte_lichen, algae")
		return
	}
	filter, ferr := syntaxonAreaFilter(r, rank)
	if ferr != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, ferr.Error())
		return
	}
	// rank is NOT checked here: its allowed values are what the index carries
	// (SELECT DISTINCT rank), which only the use case can ask. It answers
	// ErrInvalidQuery naming them, and writeQueryError turns that into the same
	// 400 as the check above.
	refs, err := s.deps.Query.SyntaxaByRank(r.Context(), rank, group, filter)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, refs)
}

// includeValues are the occurrence values ?include= accepts. Fixed on
// purpose, unlike ?rank=: this set is a schema CHECK, not an extension point.
var includeValues = []string{domain.OccurrenceUncertain, domain.OccurrenceVerified}

// syntaxonAreaFilter parses ?area= and ?include= for GET /v1/syntaxa.
//
// Nothing here is silently tolerated. An ignored filter parameter is worse
// than a rejected one: the client gets an answer that looks filtered and is
// not, and nothing in the response says so.
func syntaxonAreaFilter(r *http.Request, rank string) (input.SyntaxonAreaFilter, error) {
	q := r.URL.Query()
	code := strings.TrimSpace(q.Get("area"))
	raw, given := q["include"]

	if code == "" {
		if given {
			return input.SyntaxonAreaFilter{}, fmt.Errorf(
				"include needs an area; without one it would have no effect")
		}
		return input.SyntaxonAreaFilter{}, nil
	}
	// syntaxon_distribution and syntaxon_distribution_coverage carry ONLY
	// alliance rows (measured) — formation, class and order all have none, so
	// a bare ?area= at any of those ranks would filter nothing at all and every
	// entry would fall into the unjudgeable branch, indistinguishable from a
	// real filtered answer. Naming the rank that does carry data is the
	// difference between a rejection and a riddle.
	if rank != domain.SyntaxonRankAlliance {
		return input.SyntaxonAreaFilter{}, fmt.Errorf(
			"area needs a rank that carries distribution data (today: %s); rank=%s carries none",
			domain.SyntaxonRankAlliance, rank)
	}

	include, err := parseInclude(raw, given)
	if err != nil {
		return input.SyntaxonAreaFilter{}, err
	}
	return input.SyntaxonAreaFilter{Code: code, Include: include}, nil
}

// parseInclude turns the comma-separated set into a sorted slice. Sorted so
// two requests that differ only in order are the same request.
func parseInclude(raw []string, given bool) ([]string, error) {
	if !given {
		return []string{domain.OccurrenceVerified}, nil
	}
	if len(raw) > 1 {
		return nil, fmt.Errorf("include was given %d times; which one applies would be a guess", len(raw))
	}
	out := []string{}
	for _, part := range strings.Split(raw[0], ",") {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, fmt.Errorf("include has an empty element; allowed: %s",
				strings.Join(includeValues, ", "))
		}
		if !slices.Contains(includeValues, value) {
			return nil, fmt.Errorf("include value %q is unknown; allowed: %s",
				value, strings.Join(includeValues, ", "))
		}
		if !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	slices.Sort(out)
	return out, nil
}

// validLifeFormGroup checks the one filter whose value set is fixed: it stands
// as a CHECK in the schema and is not an extension point — unlike rank, which
// deliberately has no CHECK so a further rank can be a data row.
func validLifeFormGroup(raw string) bool {
	switch raw {
	case "", domain.LifeFormPhanerogam, domain.LifeFormBryophyteLichen, domain.LifeFormAlgae:
		return true
	default:
		return false
	}
}

// handleSyntaxon answers GET /v1/syntaxon/{id}: one syntaxon with its ancestor
// path and its direct children, so a client can walk the hierarchy without
// knowing any id but the one it just clicked.
func (s *Server) handleSyntaxon(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(mux.Vars(r)["id"])
	if id == "" {
		// A path segment is not a query, so a blank one is NOT_FOUND rather
		// than INVALID_QUERY. "/v1/syntaxon/" does not match this route at
		// all; "/v1/syntaxon/%20" does, and has to land in the same place.
		s.writeError(w, http.StatusNotFound, CodeNotFound, "syntaxon not found")
		return
	}
	detail, err := s.deps.Query.Syntaxon(r.Context(), id, language(r))
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, detail)
}
