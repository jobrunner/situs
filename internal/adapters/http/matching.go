package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

const (
	matchDefaultLimit = 10
	matchMaxLimit     = 50
)

type matchBody struct {
	ConceptIDs []string `json:"concept_ids"`
	Typology   string   `json:"typology"`
	Level      *int     `json:"level"`
	Area       string   `json:"area"`
	Limit      *int     `json:"limit"`
}

// handleHabitatTypeMatch ordnet Habitattypen nach einer beobachteten
// Artenliste. Der zurueckgegebene score ist ein Log-Likelihood: belastbar ist
// die Reihenfolge, nicht der Betrag.
func (s *Server) handleHabitatTypeMatch(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBatchBodyBytes))
	dec.DisallowUnknownFields()
	var body matchBody
	if err := dec.Decode(&body); err != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, fmt.Sprintf("malformed body: %v", err))
		return
	}
	if len(body.ConceptIDs) == 0 {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "concept_ids must not be empty")
		return
	}

	limit := matchDefaultLimit
	if body.Limit != nil {
		limit = *body.Limit
		if limit < 1 || limit > matchMaxLimit {
			s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
				fmt.Sprintf("limit must be between 1 and %d", matchMaxLimit))
			return
		}
	}
	req := input.MatchRequest{
		ConceptIDs: body.ConceptIDs,
		Typology:   typologyOrDefault(body.Typology),
		Level:      3,
		Area:       body.Area,
		Limit:      limit,
	}
	if body.Level != nil {
		req.Level = *body.Level
	}

	res, err := s.deps.Query.MatchHabitatTypes(r.Context(), req)
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

// typologyOrDefault spiegelt habitatTypeKey aus habitat.go: eine leere Angabe
// faellt auf eunis@2021 zurueck.
func typologyOrDefault(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return string(domain.DefaultTypologyID)
	}
	return strings.TrimSpace(raw)
}
