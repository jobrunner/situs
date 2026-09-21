package application

import (
	"context"
	"fmt"

	"github.com/jobrunner/situs/internal/domain"
)

// hierarchyRow is one parsed line of syntaxa_hierarchy.csv.
type hierarchyRow struct {
	code, rank, name, author, parentCode, altCode string
}

// readFormations parses syntaxa_formations.csv (letter,name_en,
// life_form_group, Task 2's data/syntaxa_formations.csv) into a
// letter -> domain.Syntaxon map. A missing file is an error here — unlike
// IngestLocalizations' optional CSVs, the formations are the root of the
// whole hierarchy and their absence must stop the ingest, not silently
// leave every class without a parent.
func readFormations(ctx context.Context, dir string, rep *SyntaxaReport) (map[string]domain.Syntaxon, error) {
	formations := map[string]domain.Syntaxon{}
	skip := newRowSkipper(&rep.SkippedRows, fileFormations, "formation")
	err := readAll(ctx, dir, fileFormations, ',', []string{"letter", colNameEN, "life_form_group"}, skip,
		func(idx map[string]int, row []string, _ int) error {
			letter := row[idx["letter"]]
			formations[letter] = domain.Syntaxon{
				ID:            letter,
				Rank:          domain.SyntaxonRankFormation,
				Name:          row[idx[colNameEN]],
				Source:        domain.SyntaxonSourceEVC,
				LifeFormGroup: row[idx["life_form_group"]],
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return formations, nil
}

// readHierarchy parses syntaxa_hierarchy.csv (code,rank,name,author,
// parent_code,alt_code, produced by pipelines/eurovegchecklist since
// Task 1) into hierarchyRows. A row with a rank other than class/order/
// alliance is skipped and counted, same tolerance as every other ingest
// file in this package. A missing file, or one missing the alt_code
// column, is an error: the column is what later resolves the EEA-only
// links onto FloraVeg's primary codes.
func readHierarchy(ctx context.Context, dir string, rep *SyntaxaReport) ([]hierarchyRow, error) {
	var rows []hierarchyRow
	skip := newRowSkipper(&rep.SkippedRows, fileHierarchy, "syntaxon hierarchy")
	err := readAll(ctx, dir, fileHierarchy, ',',
		[]string{colCode, "rank", colName, "author", "parent_code", "alt_code"}, skip,
		func(idx map[string]int, row []string, line int) error {
			rank := row[idx["rank"]]
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
				altCode:    row[idx["alt_code"]],
			})
			return nil
		})
	if err != nil {
		return nil, err
	}
	return rows, nil
}
