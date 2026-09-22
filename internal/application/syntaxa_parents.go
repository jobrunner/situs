// syntaxa_parents.go closes the parent-related gaps IngestSyntaxa leaves
// open: the EEA-only rows writeEunisOnly wrote without a parent
// (assignRemainingParents and friends), and checkDanglingParents, which
// catches a hierarchy row whose parent_code points nowhere. It is its own
// file because these are one self-contained unit, and syntaxa_ingest.go
// would otherwise run over the per-file complexity-sum ratchet.
package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// checkDanglingParents records, into rep.Orphans, every non-class hierarchy
// row (order or alliance) whose parentID is empty or was never written —
// either because the CSV's own parent_code is stale, or because the parent
// row was a class skipped for an unknown formation letter (writeHierarchy has
// no reason to know a class was dropped elsewhere in the same CSV). Class
// rows are excluded: their parent (a formation) is already validated inside
// writeHierarchy before the row is written at all, so a written class always
// has a real parent. Lives here, not in syntaxa_ingest.go, for the same
// reason this whole file does: keeping the per-file complexity sum clear of
// the codecharta ratchet's cap.
func checkDanglingParents(rows []hierarchyRow, written map[string]bool, rep *SyntaxaReport) {
	for _, r := range rows {
		if r.rank == domain.SyntaxonRankClass || !written[r.code] {
			continue
		}
		if r.parentCode == "" || !written[r.parentCode] {
			rep.Orphans = append(rep.Orphans, r.code)
		}
	}
}

// eunisOnlyRow is one FloraVeg-less syntaxon writeEunisOnly wrote (id and
// its EUNIS combi-name). It is the only input assignRemainingParents
// touches — formations and FloraVeg hierarchy rows already carry a parent
// by construction.
type eunisOnlyRow struct {
	id, name string
}

// assignRemainingParents resolves eunisOnly's parent in three tries: a
// name match against the FloraVeg alliance rows, then unanimous sibling
// consensus within its EEA order group, then giving up and recording an
// orphan. parents maps EEA id -> parent id, seeded from every hierarchy
// row's eea code.
//
// Name matching runs as its OWN first pass, over every row, before any
// sibling consensus is attempted. eunisOnly's order is the source CSV's
// name-sorted order, not the EEA id order — a single combined pass would
// make a row's sibling try depend on whether an earlier-in-the-file
// sibling happened to resolve by name first (e.g. CRU-03C/CRU-03B sort
// before CRU-03A, whose name match would have given them a unanimous
// parent). Splitting into two passes makes the sibling try see every
// name-matched parent regardless of file order.
func assignRemainingParents(tx output.IngestTx, eunisOnly []eunisOnlyRow, rows []hierarchyRow, rep *SyntaxaReport) error {
	var allianceRows []hierarchyRow
	parents := map[string]string{}
	for _, r := range rows {
		if r.rank == domain.SyntaxonRankAlliance {
			allianceRows = append(allianceRows, r)
		}
		if r.eeaCode != "" {
			parents[r.eeaCode] = r.parentCode
		}
	}

	unresolved, err := matchByName(tx, eunisOnly, allianceRows, parents, rep)
	if err != nil {
		return err
	}
	if err := matchBySibling(tx, unresolved, parents, rep); err != nil {
		return err
	}
	sort.Strings(rep.AmbiguousMatches)
	return nil
}

// matchByName is pass 1: every eunisOnly row tried against the FloraVeg
// alliance names. Rows that do not resolve here (no match, or an ambiguous
// one) are returned for the sibling-consensus pass.
func matchByName(tx output.IngestTx, eunisOnly []eunisOnlyRow, allianceRows []hierarchyRow,
	parents map[string]string, rep *SyntaxaReport) ([]eunisOnlyRow, error) {
	unresolved := make([]eunisOnlyRow, 0, len(eunisOnly))
	for _, row := range eunisOnly {
		match, ambiguous := longestPrefixMatch(row.name, allianceRows)
		if ambiguous {
			rep.AmbiguousMatches = append(rep.AmbiguousMatches, row.id)
		}
		if ambiguous || match == nil {
			unresolved = append(unresolved, row)
			continue
		}
		if err := setResolvedParent(tx, row.id, match.parentCode, domain.ParentProvenanceOfficial, parents, rep); err != nil {
			return nil, err
		}
	}
	return unresolved, nil
}

// matchBySibling is pass 2: every row pass 1 left unresolved tried against
// its EEA order group's unanimous consensus. A row still unresolved here is
// an orphan.
func matchBySibling(tx output.IngestTx, unresolved []eunisOnlyRow, parents map[string]string, rep *SyntaxaReport) error {
	for _, row := range unresolved {
		sibling := siblingConsensus(row.id, parents)
		if sibling == "" {
			rep.Orphans = append(rep.Orphans, row.id)
			continue
		}
		if err := setResolvedParent(tx, row.id, sibling, domain.ParentProvenanceDerived, parents, rep); err != nil {
			return err
		}
	}
	return nil
}

// setResolvedParent writes a resolved parent, folds it back into parents so
// a later row in the SAME pass can use it as a sibling too, and counts it.
func setResolvedParent(tx output.IngestTx, id, parentID, provenance string, parents map[string]string, rep *SyntaxaReport) error {
	if err := tx.SetSyntaxonParent(id, parentID, provenance); err != nil {
		return fmt.Errorf("setting parent of %s: %w", id, err)
	}
	parents[id] = parentID
	if provenance == domain.ParentProvenanceOfficial {
		rep.ParentsByName++
	} else {
		rep.ParentsDerived++
	}
	return nil
}

// longestPrefixMatch finds the FloraVeg alliance whose name is the longest
// prefix of eunisName. Two candidates tied at the same longest length are
// reported as ambiguous (match == nil, ambiguous == true) — never guessed.
// The prefix must end at a word boundary: either c.name is the whole string,
// or the next rune in eunisName is a space — a raw string-prefix match
// (e.g. "Salicion alba" inside "Salicion albae Soó 1930") is not a name match.
func longestPrefixMatch(eunisName string, candidates []hierarchyRow) (match *hierarchyRow, ambiguous bool) {
	bestLen := -1
	var best *hierarchyRow
	tie := false
	for i := range candidates {
		c := &candidates[i]
		if c.name == "" || !strings.HasPrefix(eunisName, c.name) {
			continue
		}
		if len(eunisName) > len(c.name) && eunisName[len(c.name)] != ' ' {
			continue
		}
		l := len(c.name)
		if l > bestLen {
			bestLen, best, tie = l, c, false
		} else if l == bestLen {
			tie = true
		}
	}
	if tie {
		return nil, true
	}
	return best, false
}

// siblingConsensus returns the parent that ALL siblings of id's EEA order
// group unanimously point to — or "", if the group is contradictory or
// empty. The group is the id without its last letter ("NAR-01E" ->
// "NAR-01").
//
// Measured over all 287 EEA order groups: 285 consistent, one
// contradictory (QUI-01, without an orphan), one with no parent set at
// all (THE-01, whose orphan the eea-code join resolves). The derivation
// is thus the tightest rule that closes the ten open cases without
// guessing.
func siblingConsensus(id string, parents map[string]string) string {
	group, ok := eeaGroup(id)
	if !ok {
		return ""
	}
	var found string
	for other, parent := range parents {
		if other == id || parent == "" {
			continue
		}
		if g, ok := eeaGroup(other); !ok || g != group {
			continue
		}
		if found == "" {
			found = parent
			continue
		}
		if found != parent {
			return ""
		}
	}
	return found
}

// eeaGroup strips the alliance letter off an EEA id. Only an id of the
// form XXX-NNL has a group; anything else has none, and then nothing
// gets derived.
func eeaGroup(id string) (string, bool) {
	if len(id) < 2 {
		return "", false
	}
	last, prev := id[len(id)-1], id[len(id)-2]
	if last < 'A' || last > 'Z' || prev < '0' || prev > '9' {
		return "", false
	}
	return id[:len(id)-1], true
}
