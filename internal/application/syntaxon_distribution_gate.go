package application

import (
	"context"
	"fmt"
	"slices"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// syntaxonGate decides which source ids may be written and records why the
// others were not. It carries the RANK, not just existence: the source covers
// alliances only, so a row for a class or order the index does happen to carry
// would otherwise pass an existence check and surface as a distribution fact
// on GET /v1/syntaxon/{id} for a rank the API never promises one for.
//
// Both readers share one gate, so a rejected id is reported once for the whole
// run rather than once per file.
type syntaxonGate struct {
	ranks       map[string]string
	unknown     unknownTracker
	nonAlliance unknownTracker
}

func newSyntaxonGate(ctx context.Context, repo output.Repository) (*syntaxonGate, error) {
	all, err := repo.AllSyntaxa(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing syntaxa: %w", err)
	}
	ranks := make(map[string]string, len(all))
	for _, s := range all {
		ranks[s.ID] = s.Rank
	}
	return &syntaxonGate{ranks: ranks}, nil
}

// allows reports whether a row for id may be written, recording the id under
// the reason it was rejected. Both rejections drop the row and keep the run
// going, the UnknownSyntaxa way: each is a disagreement between the
// distribution source and the independently pinned hierarchy, which this
// ingest cannot repair and must not hide.
func (g *syntaxonGate) allows(id string) bool {
	rank, ok := g.ranks[id]
	if !ok {
		g.unknown.add(id)
		return false
	}
	if rank != domain.SyntaxonRankAlliance {
		g.nonAlliance.add(id)
		return false
	}
	return true
}

// unknownTracker records an id once, however many rows it has. One unknown
// alliance can carry up to 136 rows, and reporting it 136 times would turn
// one fassung drift into a wall of noise.
type unknownTracker struct {
	seen map[string]bool
	ids  []string
}

func (u *unknownTracker) add(id string) {
	if u.seen[id] {
		return
	}
	if u.seen == nil {
		u.seen = map[string]bool{}
	}
	u.seen[id] = true
	u.ids = append(u.ids, id)
}

func (u *unknownTracker) sorted() []string {
	slices.Sort(u.ids)
	return u.ids
}
