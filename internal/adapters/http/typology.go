package httpapi

import "net/http"

// handleTypologies answers GET /v1/typologies: every classification system
// the index carries, sorted by id. It is the discovery entry point for the
// (typology, code) addressing every habitat-type route uses — without it a
// client has to guess eunis@2021 and never learns that eunis@2012 and
// annex1 exist alongside it.
func (s *Server) handleTypologies(w http.ResponseWriter, r *http.Request) {
	typologies, err := s.deps.Query.Typologies(r.Context())
	if err != nil {
		s.logger.ErrorContext(r.Context(), "listing typologies", "error", err)
		s.writeError(w, http.StatusInternalServerError, CodeInternalError, "internal error")
		return
	}
	s.writeJSON(w, http.StatusOK, typologies)
}
