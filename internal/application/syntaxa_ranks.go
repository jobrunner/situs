// syntaxa_ranks.go closes the last gap the two parent checks leave open.
// checkDanglingParents (syntaxa_parents.go) asks whether a parent was
// written, checkCycles (syntaxa_cycles.go) whether the chain reaches a
// formation at all — neither asks WHICH level it reaches on the way. An
// alliance hung directly under a class satisfies both and still breaks the
// formation -> class -> order -> alliance navigation GET /v1/syntaxon/{id}
// promises. Its own file for the same reason those two are split: keeping
// each file's complexity sum clear of the codecharta ratchet.
package application

import (
	"fmt"
	"sort"

	"github.com/jobrunner/situs/internal/domain"
)

// syntaxonRankLadder is the chain in navigation order, outermost first. The
// read side walks it from the other end (SyntaxonAncestors), which is why
// maxHierarchyChainSteps is exactly len(syntaxonRankLadder)-1.
var syntaxonRankLadder = []string{
	domain.SyntaxonRankFormation,
	domain.SyntaxonRankClass,
	domain.SyntaxonRankOrder,
	domain.SyntaxonRankAlliance,
}

// parentRankOf returns the rank a row of the given rank must have as its
// parent. A formation (nothing above it) and a rank that is not on the
// ladder at all both report false.
func parentRankOf(rank string) (string, bool) {
	for i, r := range syntaxonRankLadder {
		if r != rank {
			continue
		}
		if i == 0 {
			return "", false
		}
		return syntaxonRankLadder[i-1], true
	}
	return "", false
}

// writtenGraph records the rank and parent of every syntaxon one ingest
// transaction wrote. It is filled by the writers themselves rather than
// re-derived from the CSV rows afterwards: a class's parent is computed from
// its own code inside writeHierarchy, and the EEA-only rows' parents are
// decided later still, so any second derivation would be a copy that can
// drift from the one that actually wrote the row.
type writtenGraph struct {
	rank   map[string]string
	parent map[string]string
}

func newWrittenGraph() *writtenGraph {
	return &writtenGraph{rank: map[string]string{}, parent: map[string]string{}}
}

func (g *writtenGraph) add(id, rank, parentID string) {
	g.rank[id] = rank
	g.parent[id] = parentID
}

// setResolvedParents folds in the parents assignRemainingParents decided for
// the EEA-only rows, which writeEunisOnly could only record as empty.
// parents is that function's own lookup, keyed by id.
func (g *writtenGraph) setResolvedParents(eunisOnly []eunisOnlyRow, parents map[string]string) {
	for _, row := range eunisOnly {
		g.parent[row.id] = parents[row.id]
	}
}

// checkParentRanks records, into rep.WrongRankParents, every written row
// whose parent sits at the wrong level of the ladder — or whose own rank is
// not on it, which no correct parent can repair. A row whose parent was
// never written is skipped: that is checkDanglingParents' finding, and
// reporting it twice under two names would only obscure which check saw
// what.
func checkParentRanks(g *writtenGraph, rep *SyntaxaReport) {
	ids := make([]string, 0, len(g.rank))
	for id := range g.rank {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		rank := g.rank[id]
		if rank == domain.SyntaxonRankFormation {
			continue
		}
		want, ok := parentRankOf(rank)
		if !ok {
			rep.WrongRankParents = append(rep.WrongRankParents,
				fmt.Sprintf("%s has rank %q, which is not a rank of the hierarchy", id, rank))
			continue
		}
		parentID := g.parent[id]
		got, written := g.rank[parentID]
		if !written || got == want {
			continue
		}
		rep.WrongRankParents = append(rep.WrongRankParents,
			fmt.Sprintf("%s (%s) hangs under %s (%s), expected a %s", id, rank, parentID, got, want))
	}
}
