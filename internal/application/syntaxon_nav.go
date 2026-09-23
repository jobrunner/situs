package application

// syntaxon_nav.go holds the syntaxa navigation use cases.
//
// Its own file, not query.go: query.go sits at exactly the ratchet's
// per-function complexity cap of 10 and at a file baseline of 72
// (.codecharta-ratchet.json), and this project's practice is to split rather
// than raise a baseline — label.go and description.go were carved out of
// query.go for precisely this reason.

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

// Syntaxon answers one navigation step: the unit itself, the way up and the way
// down.
//
// lang=de overlays the German formation label on all three. One lookup covers
// the whole answer rather than one per ref: the labels of every syntaxon in one
// language are at most 25 rows.
func (q *QueryService) Syntaxon(ctx context.Context, id string, lang string) (input.SyntaxonDetail, error) {
	self, id, err := q.syntaxonByIDOrEEACode(ctx, id)
	if err != nil {
		return input.SyntaxonDetail{}, err
	}
	// A dangling parent_id or a cycle gets the same treatment as a dangling
	// habitat-type edge in SyntaxonHabitatTypes: an index defect is reported,
	// not bridged with a shortened breadcrumb trail. The repository's error
	// does not wrap ErrNotFound, so this stays an INTERNAL_ERROR for an id
	// that does exist.
	ancestors, err := q.repo.SyntaxonAncestors(ctx, id)
	if err != nil {
		return input.SyntaxonDetail{}, fmt.Errorf(
			"index is inconsistent: walking the ancestors of syntaxon %q: %w", id, err)
	}
	children, err := q.repo.SyntaxonChildren(ctx, id)
	if err != nil {
		return input.SyntaxonDetail{}, fmt.Errorf("fetching the children of syntaxon %q: %w", id, err)
	}
	count, err := q.repo.HabitatTypeCountForSyntaxon(ctx, id)
	if err != nil {
		return input.SyntaxonDetail{}, fmt.Errorf("counting the habitat types of syntaxon %q: %w", id, err)
	}

	dist, err := syntaxonDistributionOf(ctx, q.repo, id)
	if err != nil {
		return input.SyntaxonDetail{}, err
	}

	ref := syntaxonRef(self)
	ref.LifeFormGroup = lifeFormGroupOf(self, ancestors)
	detail := input.SyntaxonDetail{
		SyntaxonRef:            ref,
		Ancestors:              syntaxonRefs(ancestors),
		Children:               syntaxonRefs(children),
		DirectHabitatTypeCount: count,
		Distribution:           dist,
	}
	labels, err := q.syntaxonLabels(ctx, lang)
	if err != nil {
		return input.SyntaxonDetail{}, err
	}
	overlaySyntaxonLabel(&detail.SyntaxonRef, labels)
	overlaySyntaxonLabels(detail.Ancestors, labels)
	overlaySyntaxonLabels(detail.Children, labels)
	return detail, nil
}

// syntaxonLabels loads the German syntaxon labels, or nothing at all when no
// German was asked for. A nil map is the "no overlay" case and every consumer
// treats it as such, so there is no second code path for the untranslated
// answer.
func (q *QueryService) syntaxonLabels(ctx context.Context, lang string) (
	map[string][]domain.Localization, error,
) {
	if lang != deLang {
		return nil, nil
	}
	labels, err := q.repo.LocalizationsByEntityType(ctx, entitySyntaxon, deLang)
	if err != nil {
		return nil, fmt.Errorf("fetching the German syntaxon labels: %w", err)
	}
	return labels, nil
}

// overlaySyntaxonLabels adds the German label to every ref that has one. A ref
// without a translation keeps NameDE nil: only the 25 formations are authored,
// and letting an alliance borrow its formation's label would put a wrong name
// on the wire under the guise of a fallback.
func overlaySyntaxonLabels(refs []input.SyntaxonRef, labels map[string][]domain.Localization) {
	for i := range refs {
		overlaySyntaxonLabel(&refs[i], labels)
	}
}

func overlaySyntaxonLabel(ref *input.SyntaxonRef, labels map[string][]domain.Localization) {
	if rows, ok := labels[ref.ID]; ok {
		ref.NameDE = preferredLabel(rows)
	}
}

// syntaxonByIDOrEEACode resolves id as a syntaxon id, and falls back to a
// lookup by eea_code when that fails: FloraVeg became the primary source and
// syntaxon ids changed from the EEA-EUNIS code (e.g. PAP-01A) to the EVC
// primary code (e.g. AA01A), and a client holding the old code must still
// find the same unit. An id match, when there is one, always wins, so no
// precedence rule is needed to explain the order; and an eea_code more than
// one row carries does not produce a guessed winner either — the repository
// reports that as an index defect (INTERNAL_ERROR), not as a hit. Returns the resolved
// syntaxon and its real (primary) id, so callers navigate from there onward.
func (q *QueryService) syntaxonByIDOrEEACode(ctx context.Context, id string) (domain.Syntaxon, string, error) {
	self, err := q.repo.Syntaxon(ctx, id)
	if err == nil {
		return self, id, nil
	}
	if !errors.Is(err, output.ErrNotFound) {
		return domain.Syntaxon{}, "", translateNotFound(err, fmt.Sprintf("syntaxon %q", id))
	}
	self, eeaErr := q.repo.SyntaxonByEEACode(ctx, id)
	if eeaErr == nil {
		return self, self.ID, nil
	}
	if !errors.Is(eeaErr, output.ErrNotFound) {
		return domain.Syntaxon{}, "", translateNotFound(eeaErr, fmt.Sprintf("syntaxon %q (eea_code fallback)", id))
	}
	return domain.Syntaxon{}, "", translateNotFound(err, fmt.Sprintf("syntaxon %q", id))
}

// SyntaxaByRank lists one rank's syntaxa, optionally narrowed to a life-form
// group, and optionally filtered to one distribution area.
func (q *QueryService) SyntaxaByRank(ctx context.Context, rank, lifeFormGroup, lang string,
	filter input.SyntaxonAreaFilter) ([]input.SyntaxonRef, error) {
	if err := q.requireRank(ctx, rank); err != nil {
		return nil, err
	}
	rows, err := q.repo.SyntaxaByRank(ctx, rank, lifeFormGroup)
	if err != nil {
		return nil, fmt.Errorf("fetching syntaxa of rank %q: %w", rank, err)
	}
	refs := syntaxonRefs(rows)
	labels, err := q.syntaxonLabels(ctx, lang)
	if err != nil {
		return nil, err
	}
	overlaySyntaxonLabels(refs, labels)
	if !filter.Active() {
		return refs, nil
	}
	occurrences, covered, err := q.syntaxonAreaLookup(ctx, filter)
	if err != nil {
		return nil, err
	}
	return filterSyntaxaByArea(refs, occurrences, covered, filter.Include), nil
}

// requireRank rejects a rank the index does not carry. The allowed values come
// from the INDEX, not from a wired list: the schema deliberately has no CHECK
// on rank so a further rank can be a data row, and a fixed enum here would have
// taken that freedom back and made a new rank unfindable.
//
// The message names the ranks that would have worked. An empty list would look
// like "there are none" while meaning "you mistyped" — the same rule ?area= and
// ?vocab= already follow.
func (q *QueryService) requireRank(ctx context.Context, rank string) error {
	ranks, err := q.repo.SyntaxonRanks(ctx)
	if err != nil {
		return fmt.Errorf("listing the ranks the index carries: %w", err)
	}
	if slices.Contains(ranks, rank) {
		return nil
	}
	return fmt.Errorf("rank %q: the index carries %s: %w",
		rank, strings.Join(ranks, ", "), input.ErrInvalidQuery)
}

// syntaxonRef maps the domain entity onto the wire view. One mapper for every
// call site — query.go's syntaxaOf included — so a field added to
// domain.Syntaxon cannot reach three answers and be forgotten in the fourth.
func syntaxonRef(s domain.Syntaxon) input.SyntaxonRef {
	return input.SyntaxonRef{
		ID:               s.ID,
		Rank:             s.Rank,
		Name:             s.Name,
		Author:           s.Author,
		ParentID:         s.ParentID,
		EEACode:          s.EEACode,
		Source:           s.Source,
		ParentProvenance: s.ParentProvenance,
		LifeFormGroup:    s.LifeFormGroup,
	}
}

// syntaxonRefs keeps the slice non-nil: ancestors and children are serialized
// WITHOUT omitempty, and a nil slice would put a JSON null where the contract
// promises a list.
func syntaxonRefs(in []domain.Syntaxon) []input.SyntaxonRef {
	out := make([]input.SyntaxonRef, 0, len(in))
	for _, s := range in {
		out = append(out, syntaxonRef(s))
	}
	return out
}

// lifeFormGroupOf reports the group of the formation this syntaxon belongs to.
// The value is stored on formation rows only, so for every other rank it is
// read off the OUTERMOST ancestor — which is the formation, because ancestors
// are outermost-first. Empty stays empty: an unreachable formation is not a
// reason to invent a group.
func lifeFormGroupOf(self domain.Syntaxon, ancestors []domain.Syntaxon) string {
	if self.LifeFormGroup != "" {
		return self.LifeFormGroup
	}
	if len(ancestors) == 0 {
		return ""
	}
	return ancestors[0].LifeFormGroup
}
