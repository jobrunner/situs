package httpapi

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/jobrunner/situs/internal/domain"
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
	// rank is NOT checked here: its allowed values are what the index carries
	// (SELECT DISTINCT rank), which only the use case can ask. It answers
	// ErrInvalidQuery naming them, and writeQueryError turns that into the same
	// 400 as the check above.
	refs, err := s.deps.Query.SyntaxaByRank(r.Context(), rank, group)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, refs)
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
