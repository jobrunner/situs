// syntaxa_eea_codes.go holds the guard on the eea_code column, the join key
// between the FloraVeg hierarchy and the EEA-EUNIS rows and — through
// SyntaxonByEEACode — the migration path from the pre-reversal syntaxon ids.
// Its own file for the same reason syntaxa_parents.go and syntaxa_cycles.go
// are split off: keeping syntaxa_ingest.go's complexity sum clear of the
// codecharta ratchet's per-file cap.
package application

import (
	"fmt"
	"sort"
	"strings"
)

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
	seen := map[string]bool{}
	colliding := map[string]bool{}
	for _, r := range rows {
		if r.eeaCode == "" {
			continue
		}
		if seen[r.eeaCode] {
			colliding[r.eeaCode] = true
		}
		seen[r.eeaCode] = true
	}
	if len(colliding) == 0 {
		return nil
	}
	for code := range colliding {
		rep.EEACodeCollisions = append(rep.EEACodeCollisions, code)
	}
	sort.Strings(rep.EEACodeCollisions)
	return fmt.Errorf("%d eea codes in %s are claimed by more than one row: %s",
		len(rep.EEACodeCollisions), fileHierarchy, strings.Join(rep.EEACodeCollisions, ", "))
}
