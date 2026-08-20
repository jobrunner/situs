package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// handleHabitatType answers GET /v1/habitat-type/{typology}/{code}. The same
// route carries EUNIS and Annex I — a habitat type is always addressed by
// (typology, code), so a further classification system needs no new endpoint.
func (s *Server) handleHabitatType(w http.ResponseWriter, r *http.Request) {
	key, ok := s.habitatTypeKey(w, r)
	if !ok {
		return
	}
	detail, err := s.deps.Query.HabitatType(r.Context(), key, language(r), areaFilter(r))
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, detail)
}

// handleHabitatTypeSpecies answers
// GET /v1/habitat-type/{typology}/{code}/species?role=.
func (s *Server) handleHabitatTypeSpecies(w http.ResponseWriter, r *http.Request) {
	key, ok := s.habitatTypeKey(w, r)
	if !ok {
		return
	}
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	switch role {
	case "", input.RoleDiagnostic, input.RoleConstant, input.RoleDominant:
	default:
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery,
			"role must be one of diagnostic, constant, dominant")
		return
	}

	species, err := s.deps.Query.HabitatTypeSpecies(r.Context(), key, role, areaFilter(r))
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, species)
}

// handleSyntaxonHabitatTypes answers GET /v1/syntaxon/{id}/habitat-types — the
// m:n direction: one vegetation unit can belong to several habitat types.
func (s *Server) handleSyntaxonHabitatTypes(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	types, err := s.deps.Query.SyntaxonHabitatTypes(r.Context(), id, language(r))
	if err != nil {
		s.writeQueryError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, types)
}

// habitatTypeKey parses the (typology, code) path pair. A blank typology
// defaults to eunis@2021; a syntactically unparseable one is INVALID_QUERY and
// never reaches the use case. It writes the error envelope itself and reports
// whether the caller may continue.
func (s *Server) habitatTypeKey(w http.ResponseWriter, r *http.Request) (domain.HabitatTypeKey, bool) {
	vars := mux.Vars(r)
	raw := strings.TrimSpace(vars["typology"])
	if raw == "" {
		raw = string(domain.DefaultTypologyID)
	}
	typology, err := domain.ParseTypologyID(raw)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, err.Error())
		return domain.HabitatTypeKey{}, false
	}
	code := strings.TrimSpace(vars["code"])
	if code == "" {
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, "code is empty")
		return domain.HabitatTypeKey{}, false
	}
	return domain.HabitatTypeKey{Typology: typology, Code: code}, true
}

// areaFilter parses ?area= and ?only_in_area= into the use case's filter
// value object. area is a WGSRPD level 3 code — the frontend derives it from
// GPS, so there is no ISO mapping to build here. An unparseable
// only_in_area is treated as false rather than rejected: the filter is a
// convenience, and area alone (without only_in_area) already marks every
// entry.
func areaFilter(r *http.Request) input.AreaFilter {
	q := r.URL.Query()
	only, _ := strconv.ParseBool(q.Get("only_in_area"))
	return input.AreaFilter{
		Code:       strings.TrimSpace(q.Get("area")),
		OnlyInArea: only,
	}
}

// language picks the response language: ?lang= wins over Accept-Language, and
// anything the service has no labels for falls back to English rather than
// rejecting the request — a browser's Accept-Language is not a query error.
// The choice is additive only: name_en is served either way.
//
// An explicit ?lang= the service does not support short-circuits to English: a
// caller that asked for French must not be answered in German just because the
// browser also sent an Accept-Language header it never chose.
func language(r *http.Request) string {
	if raw := strings.TrimSpace(r.URL.Query().Get("lang")); raw != "" {
		if lang, ok := supportedLanguage(raw); ok {
			return lang
		}
		return langEN
	}
	for _, tag := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		// Drop any q-value and narrow "de-DE" to its base subtag.
		tag, _, _ = strings.Cut(tag, ";")
		base, _, _ := strings.Cut(strings.TrimSpace(tag), "-")
		if lang, ok := supportedLanguage(base); ok {
			return lang
		}
	}
	return langEN
}

const (
	langEN = "en"
	langDE = "de"
)

func supportedLanguage(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case langDE:
		return langDE, true
	case langEN:
		return langEN, true
	default:
		return "", false
	}
}

// writeQueryError maps the driving port's sentinels onto the error envelope.
// An error it cannot classify is an internal error and is logged with its
// cause; the client only ever sees the envelope.
func (s *Server) writeQueryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, input.ErrUnknownTypology), errors.Is(err, input.ErrUnknownArea):
		s.writeError(w, http.StatusBadRequest, CodeInvalidQuery, err.Error())
	case errors.Is(err, input.ErrNotFound):
		s.writeError(w, http.StatusNotFound, CodeNotFound, err.Error())
	default:
		s.writeError(w, http.StatusInternalServerError, CodeInternalError, "internal error")
		s.logger.ErrorContext(r.Context(), "query failed", "error", err, "path", r.URL.Path)
	}
}
