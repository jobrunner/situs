package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
	"github.com/jobrunner/situs/internal/ports/output"
)

// QueryService answers the read API from the local index alone. It translates
// the repository's ErrNotFound into the driving port's sentinels so the HTTP
// adapter can classify an answer without importing a driven port.
type QueryService struct {
	repo output.Repository
}

func NewQueryService(repo output.Repository) *QueryService {
	return &QueryService{repo: repo}
}

// HabitatType returns one type with its species, syntaxa and crosswalks.
// filter marks (and, if OnlyInArea, prunes) the species by area.
func (q *QueryService) HabitatType(ctx context.Context, key domain.HabitatTypeKey, lang string, filter input.AreaFilter) (input.HabitatTypeDetail, error) {
	if err := q.requireTypology(ctx, key.Typology); err != nil {
		return input.HabitatTypeDetail{}, err
	}
	summary, err := q.summary(ctx, key, lang)
	if err != nil {
		return input.HabitatTypeDetail{}, err
	}

	roles, err := q.repo.SpeciesRoles(ctx, key, "")
	if err != nil {
		return input.HabitatTypeDetail{}, fmt.Errorf("fetching species of %s: %w", key, err)
	}
	var ids []string
	if filter.Active() {
		ids = conceptIDsOf(roles)
	}
	areas, err := q.areaLookup(ctx, filter, ids)
	if err != nil {
		return input.HabitatTypeDetail{}, err
	}
	syntaxa, err := q.syntaxaOf(ctx, key)
	if err != nil {
		return input.HabitatTypeDetail{}, err
	}
	crosswalks, err := q.crosswalksOf(ctx, key)
	if err != nil {
		return input.HabitatTypeDetail{}, err
	}

	species := groupByRole(roles)
	for role, entries := range species {
		species[role] = markAndFilter(entries, areas, filter)
	}

	return input.HabitatTypeDetail{
		HabitatTypeSummary: summary,
		Species:            species,
		Syntaxa:            syntaxa,
		Crosswalks:         crosswalks,
	}, nil
}

// SpeciesHabitatTypes answers the excursion app's main question: given a
// recorded plant (already a concept), which habitat types does it characterize?
func (q *QueryService) SpeciesHabitatTypes(ctx context.Context, conceptID, lang string, filter input.AreaFilter) ([]input.HabitatTypeRole, error) {
	roles, err := q.repo.SpeciesRolesByConcept(ctx, conceptID)
	if err != nil {
		return nil, fmt.Errorf("fetching habitat types of concept %q: %w", conceptID, err)
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("concept %q: %w", conceptID, input.ErrNotFound)
	}
	areas, err := q.areaLookup(ctx, filter, []string{conceptID})
	if err != nil {
		return nil, err
	}
	inA := inArea(areas, conceptID, filter.Code)
	if filter.OnlyInArea && inA != nil && !*inA {
		return []input.HabitatTypeRole{}, nil
	}

	out := make([]input.HabitatTypeRole, 0, len(roles))
	for _, r := range roles {
		summary, err := q.summary(ctx, r.Key, lang)
		if err != nil {
			// A species role pointing at a type the index does not carry is an
			// inconsistent index, not a missing answer — say so instead of
			// serving a nameless type or a 404 for the whole species.
			if errors.Is(err, input.ErrNotFound) {
				return nil, fmt.Errorf("index is inconsistent: species %q has a role in unknown habitat type %s",
					r.VerbatimName, r.Key)
			}
			return nil, err
		}
		syntaxa, err := q.syntaxaOf(ctx, r.Key)
		if err != nil {
			return nil, err
		}
		out = append(out, input.HabitatTypeRole{
			HabitatTypeSummary: summary,
			Role:               r.Role,
			Fidelity:           r.Fidelity,
			Constancy:          r.Constancy,
			Syntaxa:            syntaxa,
			InArea:             inA,
		})
	}
	return out, nil
}

// HabitatTypeSpecies returns a type's species list, filtered by role when role
// is non-empty, and marked (or pruned) by area per filter.
func (q *QueryService) HabitatTypeSpecies(ctx context.Context, key domain.HabitatTypeKey, role string, filter input.AreaFilter) ([]input.SpeciesEntry, error) {
	if err := q.requireTypology(ctx, key.Typology); err != nil {
		return nil, err
	}
	if _, err := q.repo.HabitatType(ctx, key); err != nil {
		return nil, translateNotFound(err, fmt.Sprintf("habitat type %s", key))
	}
	roles, err := q.repo.SpeciesRoles(ctx, key, role)
	if err != nil {
		return nil, fmt.Errorf("fetching species of %s: %w", key, err)
	}
	var ids []string
	if filter.Active() {
		ids = conceptIDsOf(roles)
	}
	areas, err := q.areaLookup(ctx, filter, ids)
	if err != nil {
		return nil, err
	}
	out := make([]input.SpeciesEntry, 0, len(roles))
	for _, r := range roles {
		out = append(out, speciesEntry(r))
	}
	return markAndFilter(out, areas, filter), nil
}

// SyntaxonHabitatTypes shows the m:n side: the same vegetation unit can belong
// to several habitat types.
func (q *QueryService) SyntaxonHabitatTypes(ctx context.Context, syntaxonID, lang string) ([]input.HabitatTypeSummary, error) {
	if _, err := q.repo.Syntaxon(ctx, syntaxonID); err != nil {
		return nil, translateNotFound(err, fmt.Sprintf("syntaxon %q", syntaxonID))
	}
	keys, err := q.repo.HabitatTypeKeysForSyntaxon(ctx, syntaxonID)
	if err != nil {
		return nil, fmt.Errorf("fetching habitat types of syntaxon %q: %w", syntaxonID, err)
	}
	out := make([]input.HabitatTypeSummary, 0, len(keys))
	for _, key := range keys {
		summary, err := q.summary(ctx, key, lang)
		if err != nil {
			if errors.Is(err, input.ErrNotFound) {
				return nil, fmt.Errorf("index is inconsistent: syntaxon %q is linked to unknown habitat type %s",
					syntaxonID, key)
			}
			return nil, err
		}
		out = append(out, summary)
	}
	return out, nil
}

// requireTypology rejects a query about a classification system the index does
// not carry. It is a malformed question (INVALID_QUERY), not a missing answer.
func (q *QueryService) requireTypology(ctx context.Context, id domain.TypologyID) error {
	if _, err := q.repo.Typology(ctx, id); err != nil {
		if errors.Is(err, output.ErrNotFound) {
			return fmt.Errorf("typology %s: %w", id, input.ErrUnknownTypology)
		}
		return fmt.Errorf("fetching typology %s: %w", id, err)
	}
	return nil
}

// summary loads a type and overlays its localized label. The overlay is
// additive: NameEN always stays the identity.
func (q *QueryService) summary(ctx context.Context, key domain.HabitatTypeKey, lang string) (input.HabitatTypeSummary, error) {
	h, err := q.repo.HabitatType(ctx, key)
	if err != nil {
		return input.HabitatTypeSummary{}, translateNotFound(err, fmt.Sprintf("habitat type %s", key))
	}
	s := input.HabitatTypeSummary{
		Typology: key.Typology,
		Code:     key.Code,
		Level:    h.Level,
		NameEN:   h.NameEN,
		Priority: h.Priority,
	}
	if lang == deLang {
		labels, err := q.repo.Localization(ctx, "habitat_type", key.String(), deLang, nameField)
		if err != nil {
			return input.HabitatTypeSummary{}, fmt.Errorf("fetching German label of %s: %w", key, err)
		}
		s.NameDE, s.NameDEProvenance = preferredLabel(labels)
	}
	return s, nil
}

func (q *QueryService) syntaxaOf(ctx context.Context, key domain.HabitatTypeKey) ([]input.SyntaxonRef, error) {
	syntaxa, err := q.repo.Syntaxa(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("fetching syntaxa of %s: %w", key, err)
	}
	out := make([]input.SyntaxonRef, 0, len(syntaxa))
	for _, s := range syntaxa {
		out = append(out, input.SyntaxonRef{ID: s.ID, Rank: s.Rank, Name: s.Name})
	}
	return out, nil
}

// crosswalksOf turns the stored rows into the far side seen from key: a row
// stored as key -> X reads as X with its qualifier, a row stored as X -> key
// reads as X with the inverse qualifier.
func (q *QueryService) crosswalksOf(ctx context.Context, key domain.HabitatTypeKey) ([]input.CrosswalkRef, error) {
	crosswalks, err := q.repo.Crosswalks(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("fetching crosswalks of %s: %w", key, err)
	}
	out := make([]input.CrosswalkRef, 0, len(crosswalks))
	for _, c := range crosswalks {
		ref := input.CrosswalkRef{Typology: c.To.Typology, Code: c.To.Code, Qualifier: c.Qualifier}
		if c.To == key {
			ref = input.CrosswalkRef{Typology: c.From.Typology, Code: c.From.Code, Qualifier: c.Qualifier.Inverse()}
		}
		out = append(out, ref)
	}
	return out, nil
}

// groupByRole buckets species by role. The three known roles are always
// present, empty where there is nothing; a role the ingest did not expect gets
// its own bucket rather than being dropped.
func groupByRole(roles []domain.SpeciesRole) map[string][]input.SpeciesEntry {
	out := map[string][]input.SpeciesEntry{
		input.RoleDiagnostic: {},
		input.RoleConstant:   {},
		input.RoleDominant:   {},
	}
	for _, r := range roles {
		out[r.Role] = append(out[r.Role], speciesEntry(r))
	}
	return out
}

func speciesEntry(r domain.SpeciesRole) input.SpeciesEntry {
	e := input.SpeciesEntry{
		VerbatimName: r.VerbatimName,
		Role:         r.Role,
		Fidelity:     r.Fidelity,
		Constancy:    r.Constancy,
	}
	if r.ConceptID != nil {
		e.ConceptID = *r.ConceptID
	}
	return e
}

// preferredLabel picks the label to serve and reports its provenance, ordered
// official > curated > derived: the most authoritative wording wins, and the
// answer never depends on row order. Repository.Localization does order its
// rows, but this holds regardless of whether an implementation does.
func preferredLabel(labels []domain.Localization) (string, string) {
	best := map[string]domain.Localization{}
	for _, l := range labels {
		if _, seen := best[l.Provenance]; !seen {
			best[l.Provenance] = l
		}
	}
	for _, provenance := range []string{provenanceOfficial, provenanceCurated, provenanceDerived} {
		if l, ok := best[provenance]; ok {
			return l.Value, l.Provenance
		}
	}
	return "", ""
}

// translateNotFound maps the repository's ErrNotFound onto the driving port's
// sentinel, keeping the context of what was missing.
func translateNotFound(err error, what string) error {
	if errors.Is(err, output.ErrNotFound) {
		return fmt.Errorf("%s: %w", what, input.ErrNotFound)
	}
	return fmt.Errorf("fetching %s: %w", what, err)
}

// indexBackbone is the concept-id prefix the index was built from. Anything
// else cannot match and is reported as such instead of answering empty. It is a
// constant, not something derived per request: deriving it would cost a query on
// every batch call, and an index mixing backbones is not a state this design
// supports. GET /v1/info reports what the index actually holds, so a mismatch
// stays observable.
const indexBackbone = "wcvp"

// IndexInfo measures what the index holds. Nothing here is configured: a
// client's whole reason to ask is to find out whether *this* index can answer
// its concept ids, and a configured claim could not tell it that.
func (q *QueryService) IndexInfo(ctx context.Context) (input.IndexInfo, error) {
	ids, err := q.repo.ConceptIDs(ctx)
	if err != nil {
		return input.IndexInfo{}, fmt.Errorf("listing concept ids: %w", err)
	}
	areas, err := q.repo.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3)
	if err != nil {
		return input.IndexInfo{}, fmt.Errorf("listing known area codes: %w", err)
	}
	return input.IndexInfo{
		ConceptBackbones:   backbonesOf(ids),
		SpeciesWithConcept: len(ids),
		AreaScheme:         domain.SchemeWGSRPDL3,
		AreasWithData:      len(areas),
	}, nil
}

// backbonesOf reduces concept ids to their distinct prefixes, sorted so the
// answer does not depend on row order. An id without a prefix is reported as
// the empty-string backbone rather than hidden — a malformed id in the index is
// something the operator should see.
func backbonesOf(conceptIDs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range conceptIDs {
		prefix, _, _ := strings.Cut(id, ":")
		if !seen[prefix] {
			seen[prefix] = true
			out = append(out, prefix)
		}
	}
	slices.Sort(out)
	return out
}

// SpeciesSetHabitatTypes answers a whole field record at once — one entry per
// input concept id, in input order, duplicates included. It is autark: no
// verbatim name is resolved anywhere on this path.
//
// The area filter marks each entry with InArea but never drops one: the caller
// pairs response[i] with conceptIDs[i], so an entry must not go missing.
func (q *QueryService) SpeciesSetHabitatTypes(ctx context.Context, conceptIDs []string, lang string, filter input.AreaFilter) ([]input.ConceptResolution, error) {
	areas, err := q.areaLookup(ctx, filter, conceptIDs)
	if err != nil {
		return nil, err
	}

	// Deduplicated index work, one answer per input: the caller pairs
	// response[i] with conceptIDs[i], so duplicates must keep their positions.
	cache := map[string][]input.HabitatTypeRole{}
	out := make([]input.ConceptResolution, 0, len(conceptIDs))
	for _, id := range conceptIDs {
		entry := input.ConceptResolution{
			ConceptID:    id,
			HabitatTypes: []input.HabitatTypeRole{},
			InArea:       inArea(areas, id, filter.Code),
		}
		if !strings.HasPrefix(id, indexBackbone+":") {
			entry.Reason = input.ReasonUnknownBackbone
			out = append(out, entry)
			continue
		}
		types, ok := cache[id]
		if !ok {
			types, err = q.SpeciesHabitatTypes(ctx, id, lang, input.AreaFilter{})
			// The right backbone but no facts is a normal answer, not a failure.
			if err != nil && !errors.Is(err, input.ErrNotFound) {
				return nil, err
			}
			if types == nil {
				types = []input.HabitatTypeRole{}
			}
			cache[id] = types
		}
		if len(types) == 0 {
			entry.Reason = input.ReasonUnknownConcept
			out = append(out, entry)
			continue
		}
		entry.Known = true
		entry.HabitatTypes = types
		out = append(out, entry)
	}
	return out, nil
}
