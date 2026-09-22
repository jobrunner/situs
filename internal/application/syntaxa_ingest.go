// syntaxa_ingest.go is the only place syntaxa enter the index. The steps
// used to live in two functions and two transactions, because name
// matching had to find the EUNIS rows first; now that FloraVeg is the
// primary source, the dependency is reversed, and the split was only an
// opportunity to get the ordering wrong.
package application

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

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
	// and are not a formation, PLUS any row caught in a parent_id cycle
	// (checkCycles in syntaxa_cycles.go) — a cycle passes the plain
	// "was the parent written" check, since every row in it was written.
	// A non-empty value fails the ingest: a chain that breaks, or loops,
	// at one point makes the orientation service worthless at exactly
	// that point.
	Orphans []string

	// EEACodeCollisions names every eea_code that more than one hierarchy
	// row claims, sorted and each listed once. pipelines/eurovegchecklist/
	// xlsx_to_csv.py aborts on such a collision, but the Go ingest reads the
	// CSV directly, so a hand-edited or differently-produced file reaches
	// here unguarded. A listed code is excluded from the EEA -> primary-code
	// map entirely: crowning the last row read would hang an EEA unit's
	// habitat edges on an arbitrary syntaxon.
	EEACodeCollisions []string

	// AmbiguousMatches, UnknownLinkTargets and SkippedRows are the counters
	// left of the "collected but never enforced" kind this report used to
	// carry two more of (SkippedUnknownSection, SkippedPattern, both
	// removed): a class with an unknown formation letter and a code matching
	// no rank pattern both already land in SkippedRows; there never was a
	// second, finer-grained bucket that anything filled.
	AmbiguousMatches   []string
	UnknownLinkTargets []string
	SkippedRows        int
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
// its per-file sum: clear, formations, the FloraVeg hierarchy, the EEA-only
// remainder, then the habitat-type edges. written collects every syntaxon
// id this transaction wrote so writeLinks can tell an edge to a real
// syntaxon from one to nothing at all.
//
// Clearing first, not last: the two source files are the complete truth
// about the hierarchy, so a row absent from them this run must not survive
// it (see output.IngestTx.ClearSyntaxa). Doing it inside this same
// transaction, before any write, keeps the replacement atomic — a failure
// anywhere below (a dangling parent, a cycle, an orphan) rolls the delete
// back too, leaving the index exactly as it was.
func writeSyntaxa(ctx context.Context, tx output.IngestTx, dir string,
	formations map[string]domain.Syntaxon, rows []hierarchyRow, rep *SyntaxaReport) error {
	if err := tx.ClearSyntaxa(); err != nil {
		return fmt.Errorf("clearing syntaxa before ingest: %w", err)
	}
	written := map[string]bool{}
	if err := writeFormations(tx, formations, written, rep); err != nil {
		return err
	}
	if err := writeHierarchy(tx, rows, formations, written, rep); err != nil {
		return err
	}
	// A class whose formation letter is unknown is skipped (SkippedRows,
	// above) but its own order and alliance rows are written regardless —
	// writeHierarchy has no reason to know a class was dropped elsewhere in
	// the same CSV. Anything that still points nowhere after every hierarchy
	// row is written is exactly what Orphans exists to catch: it is not
	// limited to the EEA-only remainder (Step 5, below).
	checkDanglingParents(rows, written, rep)
	checkCycles(rows, written, rep)

	// Step 3: the EEA units that FloraVeg does not carry. Every other
	// unit is already represented by its FloraVeg row — writing it a
	// second time would put the same syntaxon into the index under two
	// ids.
	byEEA := mapByEEACode(rows, rep)
	eunisOnly, err := writeEunisOnly(ctx, tx, dir, byEEA, written, rep)
	if err != nil {
		return err
	}
	// Step 4: the edges. An EEA syntaxon id with a FloraVeg counterpart
	// gets resolved to the primary code; one without stays as it is.
	if err := writeLinks(ctx, tx, dir, byEEA, written, rep); err != nil {
		return err
	}
	// Step 5: the remaining EEA-only rows' parents — name match, then
	// sibling consensus, then orphan.
	if err := assignRemainingParents(tx, eunisOnly, rows, rep); err != nil {
		return err
	}
	// A chain that breaks at one point makes the orientation service
	// worthless at exactly that point — that must not be a warning one
	// skims past.
	if len(rep.Orphans) > 0 {
		sort.Strings(rep.Orphans)
		return fmt.Errorf("%d syntaxa have no parent after every step: %s",
			len(rep.Orphans), strings.Join(rep.Orphans, ", "))
	}
	return nil
}

// mapByEEACode maps each eea_code the hierarchy carries to its FloraVeg
// primary code. A code claimed by more than one row is REMOVED from the map
// rather than resolved to one of them: which row wins would be the file's
// line order, and writeLinks/writeEunisOnly would silently move an EEA
// unit's identity and its habitat edges onto an arbitrary syntaxon. Without
// the entry the EEA unit stays a row of its own, exactly like one that has
// no counterpart at all.
func mapByEEACode(rows []hierarchyRow, rep *SyntaxaReport) map[string]string {
	byEEA := map[string]string{}
	colliding := map[string]bool{}
	for _, r := range rows {
		if r.eeaCode == "" {
			continue
		}
		if _, seen := byEEA[r.eeaCode]; seen {
			delete(byEEA, r.eeaCode)
			colliding[r.eeaCode] = true
		}
		if colliding[r.eeaCode] {
			slog.Warn("eea_code claimed by more than one hierarchy row",
				"eea_code", r.eeaCode, "code", r.code, "file", fileHierarchy)
			continue
		}
		byEEA[r.eeaCode] = r.code
	}
	for code := range colliding {
		rep.EEACodeCollisions = append(rep.EEACodeCollisions, code)
	}
	sort.Strings(rep.EEACodeCollisions)
	return byEEA
}

// writeFormations writes the 25 EuroVegChecklist sections as the root of
// the hierarchy: no parent, source evc, the life-form group from the file.
// Sorted by letter so a run is reproducible.
func writeFormations(tx output.IngestTx, formations map[string]domain.Syntaxon, written map[string]bool, rep *SyntaxaReport) error {
	letters := make([]string, 0, len(formations))
	for letter := range formations {
		letters = append(letters, letter)
	}
	sort.Strings(letters)
	for _, letter := range letters {
		if err := tx.UpsertSyntaxon(formations[letter]); err != nil {
			return fmt.Errorf("upserting formation %s: %w", letter, err)
		}
		written[letter] = true
		rep.FormationsWritten++
	}
	return nil
}

// writeHierarchy writes every FloraVeg class/order/alliance row. A class's
// parent is derived from its own code (formationOf); a class whose formation
// letter is not among the known formations is skipped and counted, never
// assigned a made-up parent. Order and alliance rows keep the parent_code
// the CSV already carries.
func writeHierarchy(tx output.IngestTx, rows []hierarchyRow, formations map[string]domain.Syntaxon, written map[string]bool, rep *SyntaxaReport) error {
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
			EEACode:          r.eeaCode,
			Source:           domain.SyntaxonSourceEVC,
			ParentProvenance: domain.ParentProvenanceOfficial,
		}
		if err := tx.UpsertSyntaxon(s); err != nil {
			return fmt.Errorf("upserting %s %s: %w", r.rank, r.code, err)
		}
		written[r.code] = true
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

// writeEunisOnly writes syntaxa.csv's rows whose id is not a key of byEEA —
// an EEA unit FloraVeg already carries under its own primary code is not
// written a second time. ParentID stays empty; Task 7 sets it by name and
// sibling consensus. Name is kept exactly as the file has it: the historical
// EUNIS combi-string with embedded authorship, never split heuristically.
func writeEunisOnly(ctx context.Context, tx output.IngestTx, dir string,
	byEEA map[string]string, written map[string]bool, rep *SyntaxaReport) ([]eunisOnlyRow, error) {
	var eunisOnly []eunisOnlyRow
	skip := newRowSkipper(&rep.SkippedRows, fileEunisSyntaxa, "eunis-only syntaxon")
	err := readAll(ctx, dir, fileEunisSyntaxa, ',', []string{"id", colRank, colName, "parent_id"}, skip,
		func(idx map[string]int, row []string, _ int) error {
			id := row[idx["id"]]
			if _, ok := byEEA[id]; ok {
				return nil
			}
			name := row[idx[colName]]
			s := domain.Syntaxon{
				ID:     id,
				Rank:   row[idx[colRank]],
				Name:   name,
				Source: domain.SyntaxonSourceEUNIS,
			}
			if err := tx.UpsertSyntaxon(s); err != nil {
				return fmt.Errorf("upserting eunis-only %s: %w", id, err)
			}
			written[id] = true
			rep.EunisOnly++
			eunisOnly = append(eunisOnly, eunisOnlyRow{id: id, name: name})
			return nil
		})
	if err != nil {
		return nil, err
	}
	return eunisOnly, nil
}

// writeLinks writes habitat_type_syntaxa.csv's edges. A target that is a key
// of byEEA is resolved to its FloraVeg primary code before writing — every
// edge in the index is written fresh in this same transaction (ClearSyntaxa
// emptied habitat_type_syntaxon first), so there is no separate relink step
// for a repeat ingest to worry about. A target this ingest never wrote,
// under either id, is dropped and reported instead of linking to a syntaxon
// that does not exist.
func writeLinks(ctx context.Context, tx output.IngestTx, dir string,
	byEEA map[string]string, written map[string]bool, rep *SyntaxaReport) error {
	skip := newRowSkipper(&rep.SkippedRows, fileSyntaxonLinks, "syntaxon link")
	return readAll(ctx, dir, fileSyntaxonLinks, ',', []string{colTypologyID, colCode, "syntaxon_id"}, skip,
		func(idx map[string]int, row []string, line int) error {
			typologyID, perr := domain.ParseTypologyID(row[idx[colTypologyID]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			target := row[idx["syntaxon_id"]]
			if primary, ok := byEEA[target]; ok {
				target = primary
				rep.LinksRemapped++
			}
			if !written[target] {
				rep.UnknownLinkTargets = append(rep.UnknownLinkTargets, target)
				slog.Warn("skipping edge to unknown syntaxon",
					"target", target, "file", fileSyntaxonLinks, "line", line)
				return nil
			}
			key := domain.HabitatTypeKey{Typology: typologyID, Code: row[idx[colCode]]}
			if err := tx.LinkSyntaxon(key, target); err != nil {
				return fmt.Errorf("linking %s to %s: %w", key.Code, target, err)
			}
			rep.LinksWritten++
			return nil
		})
}
