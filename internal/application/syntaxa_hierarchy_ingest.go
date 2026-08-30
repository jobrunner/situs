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
	AlliancesMatched   int // EUNIS-Verbände mit gefundenem FloraVeg-Elternteil
	AlliancesUnmatched int // EUNIS-Verbände ohne Treffer (ParentID bleibt leer)
	AmbiguousMatches   []string
}

// hierarchyRow is one parsed line of syntaxa_hierarchy.csv.
type hierarchyRow struct {
	code, rank, name, author, parentCode string
}

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

	rows, skipped, err := readHierarchyRows(csvPath)
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
func readHierarchyRows(csvPath string) ([]hierarchyRow, int, error) {
	dir, file := filepath.Split(csvPath)
	var rows []hierarchyRow
	skipped := 0
	skip := newRowSkipper(&skipped, file, "syntaxon hierarchy")
	err := readAll(context.Background(), dir, file,
		[]string{"code", "rank", "name", "author", "parent_code"}, skip,
		func(idx map[string]int, row []string, line int) error {
			rank := row[idx["rank"]]
			switch rank {
			case "class", "order", "alliance":
			default:
				skip(line, fmt.Errorf("unknown rank %q", rank))
				return nil
			}
			rows = append(rows, hierarchyRow{
				code:       row[idx["code"]],
				rank:       rank,
				name:       row[idx["name"]],
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
		case "class", "order":
			if err := tx.UpsertSyntaxon(domain.Syntaxon{
				ID: r.code, Rank: r.rank, Name: r.name, Author: r.author, ParentID: r.parentCode,
			}); err != nil {
				return SyntaxaHierarchyReport{}, fmt.Errorf("upserting %s %s: %w", r.rank, r.code, err)
			}
			if r.rank == "class" {
				rep.ClassesWritten++
			} else {
				rep.OrdersWritten++
			}
		case "alliance":
			allianceRows = append(allianceRows, r)
		}
	}

	for _, e := range existing {
		if e.Rank != "alliance" {
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
		if err := tx.UpsertSyntaxonAuthor(e.ID, match.author, match.parentCode); err != nil {
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
func longestPrefixMatch(eunisName string, candidates []hierarchyRow) (match *hierarchyRow, ambiguous bool) {
	bestLen := -1
	var best *hierarchyRow
	tie := false
	for i := range candidates {
		c := &candidates[i]
		if c.name == "" || !strings.HasPrefix(eunisName, c.name) {
			continue
		}
		l := len(c.name)
		switch {
		case l > bestLen:
			bestLen, best, tie = l, c, false
		case l == bestLen:
			tie = true
		}
	}
	if tie {
		return nil, true
	}
	return best, false
}
