// syntaxa_parents.go closes the last gap IngestSyntaxa leaves open: the
// EEA-only rows writeEunisOnly wrote without a parent. It is its own file
// because longestPrefixMatch, siblingConsensus and eeaGroup are one
// self-contained unit, and syntaxa_ingest.go would otherwise run over the
// per-file complexity-sum ratchet.
package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

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
// row's alt code; a parent this function sets is folded back in
// immediately so a later row in the same pass can use it as a sibling too.
func assignRemainingParents(tx output.IngestTx, eunisOnly []eunisOnlyRow, rows []hierarchyRow, rep *SyntaxaReport) error {
	var allianceRows []hierarchyRow
	parents := map[string]string{}
	for _, r := range rows {
		if r.rank == domain.SyntaxonRankAlliance {
			allianceRows = append(allianceRows, r)
		}
		if r.altCode != "" {
			parents[r.altCode] = r.parentCode
		}
	}

	for _, row := range eunisOnly {
		parentID, provenance, ambiguous := resolveParent(row, allianceRows, parents)
		if ambiguous {
			rep.AmbiguousMatches = append(rep.AmbiguousMatches, row.id)
		}
		if parentID == "" {
			rep.Orphans = append(rep.Orphans, row.id)
			continue
		}
		if err := tx.SetSyntaxonParent(row.id, parentID, provenance); err != nil {
			return fmt.Errorf("setting parent of %s: %w", row.id, err)
		}
		parents[row.id] = parentID
		if provenance == domain.ParentProvenanceOfficial {
			rep.ParentsByName++
		} else {
			rep.ParentsDerived++
		}
	}
	sort.Strings(rep.AmbiguousMatches)
	return nil
}

// resolveParent tries the name match first, then sibling consensus.
// ambiguous and a found parentID are NOT mutually exclusive: an ambiguous
// name match is never guessed from, but it does not stop the sibling try
// either, because the ambiguity is purely a property of the name path —
// siblings find each other over the alt code, which does not care whether
// two FloraVeg names happen to tie in length. Refusing the sibling's
// unanimous answer just because the name path also (independently) saw two
// equally-long candidates would trade a real answer for an avoidable
// orphan, and a broken parent chain is the worse failure of the two. The
// ambiguity is still reported in AmbiguousMatches either way — resolving
// via sibling consensus does not hide it, it only stops it from being a
// dead end on its own.
func resolveParent(row eunisOnlyRow, allianceRows []hierarchyRow, parents map[string]string) (parentID, provenance string, ambiguous bool) {
	match, amb := longestPrefixMatch(row.name, allianceRows)
	if !amb && match != nil {
		return match.parentCode, domain.ParentProvenanceOfficial, false
	}
	if sibling := siblingConsensus(row.id, parents); sibling != "" {
		return sibling, domain.ParentProvenanceDerived, amb
	}
	return "", "", amb
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
// all (THE-01, whose orphan the alt-code join resolves). The derivation
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
