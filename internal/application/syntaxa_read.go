package application

import (
	"context"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
)

// hierarchyRow is one parsed line of syntaxa_hierarchy.csv.
type hierarchyRow struct {
	code, rank, name, author, parentCode, eeaCode string
}

// readFormations parses syntaxa_formations.csv (letter,name_en,
// life_form_group, Task 2's data/syntaxa_formations.csv) into a
// letter -> domain.Syntaxon map. A missing file is an error here — unlike
// IngestLocalizations' optional CSVs, the formations are the root of the
// whole hierarchy and their absence must stop the ingest, not silently
// leave every class without a parent.
//
// The letter is a key, so it is validated as one: empty is rejected (it
// would write a formation with the id "" that no class can point at), and a
// letter claimed twice is rejected too (the later row would silently
// overwrite the earlier one's name and life-form group). Both fail before
// the ingest's transaction opens.
//
// Deliberately NOT checked: that the set is exactly A–Y, gapless. The
// sections belong to the EuroVegChecklist fassung this file pins, not to
// this code — asserting them here would fail a legitimate later fassung that
// adds or drops one. A missing letter is also not the silent case it looks
// like: every class under it is skipped, and each of that class's orders and
// alliances then has an unwritten parent, which checkDanglingParents turns
// into an orphan and writeSyntaxa turns into an abort. Measured against the
// pinned artifacts, all 25 letters carry classes, so a truncated file always
// hits that path.
func readFormations(ctx context.Context, dir string, rep *SyntaxaReport) (map[string]domain.Syntaxon, error) {
	formations := map[string]domain.Syntaxon{}
	skip := newRowSkipper(&rep.SkippedRows, fileFormations, "formation")
	err := readAll(ctx, dir, fileFormations, ',', []string{"letter", colNameEN, "life_form_group"}, skip,
		func(idx map[string]int, row []string, line int) error {
			letter := row[idx["letter"]]
			if letter == "" {
				return fmt.Errorf("%s:%d: formation row without a letter", fileFormations, line)
			}
			if _, dup := formations[letter]; dup {
				return fmt.Errorf("%s:%d: formation letter %q is claimed twice", fileFormations, line, letter)
			}
			formations[letter] = domain.Syntaxon{
				ID:               letter,
				Rank:             domain.SyntaxonRankFormation,
				Name:             row[idx[colNameEN]],
				Source:           domain.SyntaxonSourceEVC,
				LifeFormGroup:    row[idx["life_form_group"]],
				ParentProvenance: domain.ParentProvenanceOfficial,
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	// A header-only file passes every check above and the required-file check
	// too — it exists and carries all three columns. Since writeSyntaxa
	// REPLACES the hierarchy (ClearSyntaxa), such a run would commit without a
	// single root: a truncated curated source silently deleting the whole
	// syntaxa part of the index. An empty syntaxa_hierarchy.csv stays allowed —
	// the formations alone are a bare but valid hierarchy; only the empty
	// formation set is the defect.
	if len(formations) == 0 {
		return nil, fmt.Errorf("%s carries no formation at all; that would replace the whole hierarchy with nothing", fileFormations)
	}
	return formations, nil
}

// readEunisOnly parses syntaxa.csv and returns exactly those rows that will
// be written: an EEA unit FloraVeg already carries under its own primary code
// (its id is a key of byEEA) stands in the index once already and is dropped
// here, as is a row this file cannot hold at all.
//
// Reading this before the transaction is what lets checkIDNamespace see the
// third writer into the syntaxon id namespace. It has to be the kept rows, not
// the file's rows: an id the run never writes cannot collide with anything,
// and failing on one would reject a perfectly ordinary source.
func readEunisOnly(ctx context.Context, dir string, byEEA map[string]string,
	rep *SyntaxaReport) ([]eunisOnlyRow, error) {
	var eunisOnly []eunisOnlyRow
	skip := newRowSkipper(&rep.SkippedRows, fileEunisSyntaxa, "eunis-only syntaxon")
	err := readAll(ctx, dir, fileEunisSyntaxa, ',', []string{"id", colRank, colName, "parent_id"}, skip,
		func(idx map[string]int, row []string, line int) error {
			id := row[idx["id"]]
			// The id is this row's key, checked like the distribution readers
			// check theirs: written as it stands, an empty one becomes a
			// syntaxon with the id "", and a name matching a FloraVeg alliance
			// even resolves its parent, so the run commits an invalid
			// navigation target.
			if id == "" {
				skip(line, fmt.Errorf("eunis-only row without an id"))
				return nil
			}
			// A formation is the root of the hierarchy, and only
			// syntaxa_formations.csv defines one. Written from here the row
			// would go through assignRemainingParents like any other, and a
			// name matching a FloraVeg alliance hands it an order as parent —
			// a formation WITH a parent, which checkParentRanks reports but
			// which must not be produced in the first place. Discarded and
			// reported like the row without an id above, not fatal: the row
			// claims a rank this file cannot hold, so dropping it loses
			// nothing but itself.
			rank := row[idx[colRank]]
			if rank == domain.SyntaxonRankFormation {
				skip(line, fmt.Errorf("eunis-only row %s claims rank %q, which only %s defines",
					id, rank, fileFormations))
				return nil
			}
			if _, ok := byEEA[id]; ok {
				return nil
			}
			eunisOnly = append(eunisOnly, eunisOnlyRow{id: id, rank: rank, name: row[idx[colName]]})
			return nil
		})
	if err != nil {
		return nil, err
	}
	return eunisOnly, nil
}

// readHierarchy parses syntaxa_hierarchy.csv (code,rank,name,author,
// parent_code,eea_code, produced by pipelines/eurovegchecklist since
// Task 1) into hierarchyRows. A row with a rank other than class/order/
// alliance is skipped and counted, same tolerance as every other ingest
// file in this package. A missing file, or one missing the eea_code
// column, is an error: the column is what later resolves the EEA-only
// links onto FloraVeg's primary codes.
func readHierarchy(ctx context.Context, dir string, rep *SyntaxaReport) ([]hierarchyRow, error) {
	var rows []hierarchyRow
	skip := newRowSkipper(&rep.SkippedRows, fileHierarchy, "syntaxon hierarchy")
	err := readAll(ctx, dir, fileHierarchy, ',',
		[]string{colCode, colRank, colName, "author", "parent_code", "eea_code"}, skip,
		func(idx map[string]int, row []string, line int) error {
			rank := row[idx[colRank]]
			switch rank {
			case domain.SyntaxonRankClass, domain.SyntaxonRankOrder, domain.SyntaxonRankAlliance:
			default:
				skip(line, fmt.Errorf("unknown rank %q", rank))
				return nil
			}
			// The primary code is the row's identity: written as it stands, an
			// empty one becomes a syntaxon with the id "" that nothing can
			// reach, while a valid rank and a valid parent carry it past the
			// dangling-parent, cycle and rank checks unnoticed. Skipped and
			// counted, not fatal: the row has no key anything else could point
			// at, so dropping it loses nothing but itself.
			if row[idx[colCode]] == "" {
				skip(line, fmt.Errorf("%s row without a code", rank))
				return nil
			}
			rows = append(rows, hierarchyRow{
				code:       row[idx[colCode]],
				rank:       rank,
				name:       row[idx[colName]],
				author:     row[idx["author"]],
				parentCode: row[idx["parent_code"]],
				eeaCode:    row[idx["eea_code"]],
			})
			return nil
		})
	if err != nil {
		return nil, err
	}
	return rows, nil
}
