// syntaxa_code_collisions.go holds the guards on the two code columns of
// syntaxa_hierarchy.csv: the primary EVC code, which is the syntaxon's
// identity, and the eea_code, the join key to the EEA-EUNIS rows and — through
// SyntaxonByEEACode — the migration path from the pre-reversal syntaxon ids.
// Its own file for the same reason syntaxa_parents.go and syntaxa_cycles.go
// are split off: keeping syntaxa_ingest.go's complexity sum clear of the
// codecharta ratchet's per-file cap.
package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
)

// checkIDNamespace fails the ingest when two of the three sources that write
// into the syntaxon id namespace claim one id. checkPrimaryCodeCollisions only
// covers syntaxa_hierarchy.csv against itself, but the formations, the
// hierarchy rows and the EEA-only remainder all end up in the same
// syntaxon.id, written in that order and merged by ON CONFLICT(id) DO UPDATE —
// so a hierarchy row coded "C" replaces formation C with an alliance, and if
// its own parent chain is rank-correct, every later check passes and the index
// commits with a root gone.
//
// Fatal, not discarded-and-reported, for both of the sources that could lose a
// row: the formations are the root of the navigation, so preferring a later
// writer over them is never right, and preferring the formation instead would
// silently drop a hierarchy row together with the whole subtree hanging off it.
// The id namespace is the one thing every source has to agree on, and no
// downstream step can repair a disagreement in it.
//
// eunisOnly must be the rows that are actually written (readEunisOnly's
// result), never syntaxa.csv's rows: a row with a FloraVeg counterpart never
// reaches the table and therefore cannot collide with anything.
func checkIDNamespace(formations map[string]domain.Syntaxon, rows []hierarchyRow,
	eunisOnly []eunisOnlyRow, rep *SyntaxaReport) error {
	claims := map[string]map[string]bool{}
	claim := func(id, file string) {
		if claims[id] == nil {
			claims[id] = map[string]bool{}
		}
		claims[id][file] = true
	}
	for letter := range formations {
		claim(letter, fileFormations)
	}
	for _, r := range rows {
		claim(r.code, fileHierarchy)
	}
	for _, r := range eunisOnly {
		claim(r.id, fileEunisSyntaxa)
	}
	for id, files := range claims {
		if len(files) < 2 {
			continue
		}
		named := make([]string, 0, len(files))
		for file := range files {
			named = append(named, file)
		}
		sort.Strings(named)
		rep.IDCollisions = append(rep.IDCollisions,
			fmt.Sprintf("%s (%s)", id, strings.Join(named, ", ")))
	}
	if len(rep.IDCollisions) == 0 {
		return nil
	}
	sort.Strings(rep.IDCollisions)
	return fmt.Errorf("%d syntaxon ids are claimed by more than one source: %s",
		len(rep.IDCollisions), strings.Join(rep.IDCollisions, "; "))
}

// duplicateCodes returns every value codeOf claims more than once, sorted and
// each listed once. The empty string is not a code and never collides.
func duplicateCodes(rows []hierarchyRow, codeOf func(hierarchyRow) string) []string {
	seen := map[string]bool{}
	colliding := map[string]bool{}
	for _, r := range rows {
		code := codeOf(r)
		if code == "" {
			continue
		}
		if seen[code] {
			colliding[code] = true
		}
		seen[code] = true
	}
	codes := make([]string, 0, len(colliding))
	for code := range colliding {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// checkPrimaryCodeCollisions fails the ingest when a primary code is claimed by
// more than one hierarchy row. The primary code IS the syntaxon's id, and
// UpsertSyntaxon resolves a conflict on it with ON CONFLICT(id) DO UPDATE, so a
// duplicate does not surface as an error: the later row silently replaces the
// earlier one's rank, name and parent while the report counts both rows, and
// every parent_code and habitat-type edge pointing at that code now means
// whichever row happened to come last in the file.
func checkPrimaryCodeCollisions(rows []hierarchyRow, rep *SyntaxaReport) error {
	rep.PrimaryCodeCollisions = duplicateCodes(rows, func(r hierarchyRow) string { return r.code })
	if len(rep.PrimaryCodeCollisions) == 0 {
		return nil
	}
	return fmt.Errorf("%d codes in %s are claimed by more than one row: %s",
		len(rep.PrimaryCodeCollisions), fileHierarchy, strings.Join(rep.PrimaryCodeCollisions, ", "))
}

// checkEEACodeCollisions fails the ingest when an eea_code is claimed by more
// than one hierarchy row, naming every such code so the source can be repaired
// in one pass. Two rows on one code would make the code ambiguous as a lookup
// key — and it is one: SyntaxonByEEACode serves the migration path from the
// pre-reversal ids, so an ambiguous code resolves an old id to an arbitrary
// syntaxon and to that syntaxon's habitat-type edges.
//
// Which row to prefer cannot be decided here (it would be the file's line
// order), and no downstream step can repair it either, so this runs once, up
// front, instead of each map over the eea_code column excluding the code again.
func checkEEACodeCollisions(rows []hierarchyRow, rep *SyntaxaReport) error {
	rep.EEACodeCollisions = duplicateCodes(rows, func(r hierarchyRow) string { return r.eeaCode })
	if len(rep.EEACodeCollisions) == 0 {
		return nil
	}
	return fmt.Errorf("%d eea codes in %s are claimed by more than one row: %s",
		len(rep.EEACodeCollisions), fileHierarchy, strings.Join(rep.EEACodeCollisions, ", "))
}
