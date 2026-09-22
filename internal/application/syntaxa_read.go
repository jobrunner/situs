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
	return formations, nil
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
