// syntaxa_cycles.go closes the gap checkDanglingParents (syntaxa_parents.go)
// leaves open: it only checks that a row's immediate parent was WRITTEN, so a
// cycle such as CA01 -> CA01A -> CA01 passes it cleanly — every row in the
// cycle has a written parent, just never one that reaches a formation. Its
// own file for the same reason syntaxa_parents.go and syntaxa_ingest.go are
// split: keeping each file's complexity sum clear of the codecharta ratchet.
package application

import "github.com/jobrunner/situs/internal/domain"

// maxHierarchyChainSteps mirrors maxSyntaxonAncestors in
// internal/adapters/sqlite/read_syntaxon_rank.go: alliance -> order -> class
// -> formation is three steps. The two constants guard the same invariant
// from opposite ends — this one at ingest time, before the row ever reaches
// the index; that one at read time, walking the row the ingest let through.
// They cannot share a literal (different packages, different files this
// project deliberately keeps apart), so a change to one is a change to
// consider for the other, not a change enforced by the compiler.
const maxHierarchyChainSteps = 3

// checkCycles appends to rep.Orphans every written, non-class hierarchy row
// whose parent_id chain does not reach a formation within
// maxHierarchyChainSteps hops — because it loops back onto an earlier node,
// or simply runs deeper than the promised depth. Class rows are excluded for
// the same reason checkDanglingParents excludes them: writeHierarchy derives
// a class's parent from its own code and validates it against the known
// formations before ever writing the row, so a written class can never be
// part of a cycle.
func checkCycles(rows []hierarchyRow, written map[string]bool, rep *SyntaxaReport) {
	parentOf := map[string]string{}
	for _, r := range rows {
		if written[r.code] {
			parentOf[r.code] = r.parentCode
		}
	}
	for _, r := range rows {
		if r.rank == domain.SyntaxonRankClass || !written[r.code] {
			continue
		}
		if chainCycles(r.code, parentOf) {
			rep.Orphans = append(rep.Orphans, r.code)
		}
	}
}

// chainCycles walks parentOf upward from code. Running off the map (a
// formation, or a parent checkDanglingParents already flagged as unwritten)
// ends the walk cleanly. Revisiting an already-seen node, or still having a
// parent after maxHierarchyChainSteps hops, means a cycle.
func chainCycles(code string, parentOf map[string]string) bool {
	visited := map[string]bool{code: true}
	cur := code
	for range maxHierarchyChainSteps {
		parent, ok := parentOf[cur]
		if !ok {
			return false
		}
		if visited[parent] {
			return true
		}
		visited[parent] = true
		cur = parent
	}
	_, stillChained := parentOf[cur]
	return stillChained
}
