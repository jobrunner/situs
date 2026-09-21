// syntaxa_ingest.go is the only place syntaxa enter the index. The steps
// used to live in two functions and two transactions, because name
// matching had to find the EUNIS rows first; now that FloraVeg is the
// primary source, the dependency is reversed, and the split was only an
// opportunity to get the ordering wrong.
package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

const (
	fileFormations    = "syntaxa_formations.csv"
	fileHierarchy     = "syntaxa_hierarchy.csv"
	fileEunisSyntaxa  = "syntaxa.csv"
	fileSyntaxonLinks = "habitat_type_syntaxa.csv"
)

// SyntaxaReport counts what a syntaxa ingest wrote and what it could not
// decide. Every number is measured, none is estimated.
type SyntaxaReport struct {
	FormationsWritten int
	ClassesWritten    int
	OrdersWritten     int
	AlliancesWritten  int

	EunisOnly     int
	LinksWritten  int
	LinksRemapped int

	ParentsByName  int
	ParentsDerived int

	// Orphans are rows that, after every step, remained without a parent
	// and are not a formation. A non-empty value fails the ingest: a
	// chain that breaks at one point makes the orientation service
	// worthless at exactly that point.
	Orphans []string

	AltCodeCollisions  []string
	AmbiguousMatches   []string
	UnknownLinkTargets []string
	SkippedRows        int

	// SkippedUnknownSection counts class rows whose first letter is not a
	// known formation: skipped, never invented. Filled starting Task 7's
	// remaining-parents step; Task 5 counts the same case in SkippedRows.
	SkippedUnknownSection int
	// SkippedPattern counts rows whose code fits no rank pattern. Filled
	// starting Task 7.
	SkippedPattern int
}

// formationOf returns a class's formation id: the first letter of its
// code. The level lives in the code itself (class "CA" belongs to
// section "C"), just like rank and parent — never guessed from the name.
func formationOf(classCode string) string {
	if classCode == "" {
		return ""
	}
	return classCode[:1]
}

// IngestSyntaxa loads dir's four syntaxa CSVs into repo in one transaction:
// syntaxa_formations.csv (the 25 EuroVegChecklist sections, Task 2),
// syntaxa_hierarchy.csv (the FloraVeg class/order/alliance rows, now the
// primary source), syntaxa.csv and habitat_type_syntaxa.csv (the remaining
// EEA-only rows and links, Task 6/7). Unlike IngestLocalizations, a missing
// formations or hierarchy file is an error, not "nothing to do yet": both
// are primary sources now, and an index built without them would silently
// carry no vegetation hierarchy at all.
func IngestSyntaxa(ctx context.Context, repo output.Repository, dir string) (SyntaxaReport, error) {
	var rep SyntaxaReport

	formations, err := readFormations(ctx, dir, &rep)
	if err != nil {
		return SyntaxaReport{}, err
	}
	rows, err := readHierarchy(ctx, dir, &rep)
	if err != nil {
		return SyntaxaReport{}, err
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return SyntaxaReport{}, fmt.Errorf("beginning syntaxa ingest transaction: %w", err)
	}
	if err := writeSyntaxa(ctx, tx, dir, formations, rows, &rep); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return SyntaxaReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return SyntaxaReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntaxaReport{}, fmt.Errorf("committing syntaxa ingest transaction: %w", err)
	}
	return rep, nil
}

// writeSyntaxa runs the ingest steps in order, each through its own
// function so the file's per-function complexity ratchet stays clear of
// its per-file sum: formations, then the FloraVeg hierarchy. Task 6 and 7
// append the EUNIS-only, link and remaining-parent steps here.
func writeSyntaxa(_ context.Context, tx output.IngestTx, _ string,
	formations map[string]domain.Syntaxon, rows []hierarchyRow, rep *SyntaxaReport) error {
	if err := writeFormations(tx, formations, rep); err != nil {
		return err
	}
	if err := writeHierarchy(tx, rows, formations, rep); err != nil {
		return err
	}
	return nil
}

// writeFormations writes the 25 EuroVegChecklist sections as the root of
// the hierarchy: no parent, source evc, the life-form group from the file.
// Sorted by letter so a run is reproducible.
func writeFormations(tx output.IngestTx, formations map[string]domain.Syntaxon, rep *SyntaxaReport) error {
	letters := make([]string, 0, len(formations))
	for letter := range formations {
		letters = append(letters, letter)
	}
	sort.Strings(letters)
	for _, letter := range letters {
		if err := tx.UpsertSyntaxon(formations[letter]); err != nil {
			return fmt.Errorf("upserting formation %s: %w", letter, err)
		}
		rep.FormationsWritten++
	}
	return nil
}

// writeHierarchy writes every FloraVeg class/order/alliance row. A class's
// parent is derived from its own code (formationOf); a class whose formation
// letter is not among the known formations is skipped and counted, never
// assigned a made-up parent. Order and alliance rows keep the parent_code
// the CSV already carries.
func writeHierarchy(tx output.IngestTx, rows []hierarchyRow, formations map[string]domain.Syntaxon, rep *SyntaxaReport) error {
	for _, r := range rows {
		parentID := r.parentCode
		if r.rank == domain.SyntaxonRankClass {
			formation := formationOf(r.code)
			if _, ok := formations[formation]; !ok {
				rep.SkippedRows++
				continue
			}
			parentID = formation
		}
		s := domain.Syntaxon{
			ID:               r.code,
			Rank:             r.rank,
			Name:             r.name,
			Author:           r.author,
			ParentID:         parentID,
			AltCode:          r.altCode,
			Source:           domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial,
		}
		if err := tx.UpsertSyntaxon(s); err != nil {
			return fmt.Errorf("upserting %s %s: %w", r.rank, r.code, err)
		}
		switch r.rank {
		case domain.SyntaxonRankClass:
			rep.ClassesWritten++
		case domain.SyntaxonRankOrder:
			rep.OrdersWritten++
		case domain.SyntaxonRankAlliance:
			rep.AlliancesWritten++
		}
	}
	return nil
}
