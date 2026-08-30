package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// SyntaxaHierarchyReport summarizes the FloraVeg hierarchy ingest + the
// EUNIS-alliance matching pass.
type SyntaxaHierarchyReport struct {
	ClassesWritten     int
	OrdersWritten      int
	AlliancesMatched   int // EUNIS alliances with a found FloraVeg parent
	AlliancesUnmatched int // EUNIS alliances without a match (ParentID stays empty)
	AmbiguousMatches   []string
}

// hierarchyRow is one parsed line of syntaxa_hierarchy.csv.
type hierarchyRow struct {
	code, rank, name, author, parentCode string
}

// rankAlliance is the FloraVeg/EUNIS rank that this file matches against
// existing syntaxa. rankClass exists alongside it only because "class"
// crosses goconst's occurrence threshold too; "order" stays a literal since
// it occurs only twice.
const (
	rankAlliance = "alliance"
	rankClass    = "class"
)

// IngestSyntaxaHierarchy loads csvPath (syntaxa_hierarchy.csv:
// code,rank,name,author,parent_code, produced by pipelines/eurovegchecklist)
// into repo. It writes every class/order row as a new syntaxon, then matches
// every EUNIS alliance already in the index against the FloraVeg alliance
// names by longest-prefix — a match lends Author and ParentID, a non-match
// stays untouched, an equal-length ambiguous match is never guessed.
//
// A missing csvPath is "no hierarchy data yet", not an error, mirroring
// IngestLocalizations: the caller (situs ingest) must keep working on an
// index that has no FloraVeg CSV pinned.
func IngestSyntaxaHierarchy(ctx context.Context, repo output.Repository, csvPath string) (SyntaxaHierarchyReport, error) {
	if _, err := os.Stat(csvPath); err != nil {
		if os.IsNotExist(err) {
			slog.InfoContext(ctx, "no syntaxa hierarchy file, skipping", "path", csvPath)
			return SyntaxaHierarchyReport{}, nil
		}
		return SyntaxaHierarchyReport{}, fmt.Errorf("statting %s: %w", csvPath, err)
	}

	rows, skipped, err := readHierarchyRows(ctx, csvPath)
	if err != nil {
		return SyntaxaHierarchyReport{}, err
	}

	existing, err := repo.AllSyntaxa(ctx)
	if err != nil {
		return SyntaxaHierarchyReport{}, fmt.Errorf("reading existing syntaxa: %w", err)
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return SyntaxaHierarchyReport{}, fmt.Errorf("beginning syntaxa hierarchy ingest transaction: %w", err)
	}

	rep, err := ingestHierarchyRows(tx, rows, existing)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return SyntaxaHierarchyReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return SyntaxaHierarchyReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntaxaHierarchyReport{}, fmt.Errorf("committing syntaxa hierarchy ingest transaction: %w", err)
	}

	if skipped > 0 {
		slog.WarnContext(ctx, "skipped malformed rows in syntaxa_hierarchy.csv", "skipped", skipped)
	}
	return rep, nil
}

// readHierarchyRows parses csvPath's data rows, skipping (and counting) any
// row with the wrong field count or an unrecognized rank, same tolerance as
// every other ingest file in this package.
func readHierarchyRows(ctx context.Context, csvPath string) ([]hierarchyRow, int, error) {
	dir, file := filepath.Split(csvPath)
	var rows []hierarchyRow
	skipped := 0
	skip := newRowSkipper(&skipped, file, "syntaxon hierarchy")
	err := readAll(ctx, dir, file,
		[]string{"code", "rank", colName, "author", "parent_code"}, skip,
		func(idx map[string]int, row []string, line int) error {
			rank := row[idx["rank"]]
			switch rank {
			case rankClass, "order", rankAlliance:
			default:
				skip(line, fmt.Errorf("unknown rank %q", rank))
				return nil
			}
			rows = append(rows, hierarchyRow{
				code:       row[idx["code"]],
				rank:       rank,
				name:       row[idx[colName]],
				author:     row[idx["author"]],
				parentCode: row[idx["parent_code"]],
			})
			return nil
		})
	if err != nil {
		return nil, 0, err
	}
	return rows, skipped, nil
}

// ingestHierarchyRows writes every class/order row as a new syntaxon and
// matches every already-indexed EUNIS alliance against the FloraVeg alliance
// rows by longest-name-prefix.
func ingestHierarchyRows(tx output.IngestTx, rows []hierarchyRow, existing []domain.Syntaxon) (SyntaxaHierarchyReport, error) {
	var rep SyntaxaHierarchyReport
	var allianceRows []hierarchyRow

	for _, r := range rows {
		switch r.rank {
		case rankClass, "order":
			if err := tx.UpsertSyntaxon(domain.Syntaxon{
				ID: r.code, Rank: r.rank, Name: r.name, Author: r.author, ParentID: r.parentCode,
			}); err != nil {
				return SyntaxaHierarchyReport{}, fmt.Errorf("upserting %s %s: %w", r.rank, r.code, err)
			}
			if r.rank == rankClass {
				rep.ClassesWritten++
			} else {
				rep.OrdersWritten++
			}
		case rankAlliance:
			allianceRows = append(allianceRows, r)
		}
	}

	for _, e := range existing {
		if e.Rank != rankAlliance {
			continue
		}
		match, ambiguous := longestPrefixMatch(e.Name, allianceRows)
		if ambiguous {
			rep.AmbiguousMatches = append(rep.AmbiguousMatches, e.ID)
			continue
		}
		if match == nil {
			rep.AlliancesUnmatched++
			continue
		}
		if err := tx.UpsertSyntaxonAuthor(e.ID, match.name, match.author, match.parentCode); err != nil {
			return SyntaxaHierarchyReport{}, fmt.Errorf("setting author of %s: %w", e.ID, err)
		}
		rep.AlliancesMatched++
	}
	sort.Strings(rep.AmbiguousMatches)
	return rep, nil
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
