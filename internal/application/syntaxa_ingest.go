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
	// here unguarded. A non-empty value fails the ingest, like Orphans: the
	// eea_code is the documented migration path from the old syntaxon ids,
	// so a duplicate makes GET /v1/syntaxon/{old-id} resolve to an arbitrary
	// one of the claiming rows, together with its habitat-type edges. This
	// field is the diagnosis to that abort and is returned filled with it.
	EEACodeCollisions []string

	// PrimaryCodeCollisions names every primary EVC code that more than one
	// hierarchy row claims, sorted and each listed once, and is returned filled
	// with the abort it diagnoses — like EEACodeCollisions above. The primary
	// code is the syntaxon's id, so a duplicate is worse than an ambiguous
	// lookup key: UpsertSyntaxon merges the rows into one
	// (ON CONFLICT(id) DO UPDATE), the later one winning rank, name and parent,
	// while the report still counts both. The pipeline aborts on it too, but
	// the Go ingest reads the CSV directly.
	PrimaryCodeCollisions []string

	// IDCollisions names every syntaxon id that more than one of the three
	// writing sources claims, each as "id (file, file)", sorted and listed
	// once, and is returned filled with the abort it diagnoses. It is the
	// namespace-wide counterpart to PrimaryCodeCollisions above, which only
	// sees syntaxa_hierarchy.csv against itself: formations, hierarchy rows
	// and EEA-only rows share one id column, so a collision between two of
	// them is the same DO UPDATE merge one file's duplicate is.
	IDCollisions []string

	// AmbiguousMatches, UnknownLinkTargets and SkippedRows are the counters
	// left of the "collected but never enforced" kind this report used to
	// carry two more of (SkippedUnknownSection, SkippedPattern, both
	// removed): a class with an unknown formation letter and a code matching
	// no rank pattern both already land in SkippedRows; there never was a
	// second, finer-grained bucket that anything filled.
	// WrongRankParents names every written row whose parent sits at the wrong
	// level of formation -> class -> order -> alliance (checkParentRanks in
	// syntaxa_ranks.go). A non-empty value fails the ingest, like Orphans: a
	// chain that skips a level reaches a formation and still cannot be walked
	// the way GET /v1/syntaxon/{id} promises to walk it.
	WrongRankParents []string

	AmbiguousMatches   []string
	UnknownLinkTargets []string
	SkippedRows        int
}

// syntaxaSources is everything IngestSyntaxa parsed and validated before it
// opened the transaction: the three sources that write into the syntaxon id
// namespace plus the eea_code -> primary code index derived from the
// hierarchy. Reading and checking all three up front is what makes
// checkIDNamespace possible at all — a collision between two sources cannot be
// repaired once the first of them is written.
type syntaxaSources struct {
	formations map[string]domain.Syntaxon
	rows       []hierarchyRow
	byEEA      map[string]string
	eunisOnly  []eunisOnlyRow
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
	// Before the transaction opens: nothing about a duplicate code can be
	// repaired by writing rows first. The report is returned filled here,
	// unlike the zero value every other error path returns, because its
	// collision list is exactly the diagnosis to this abort.
	if err := checkPrimaryCodeCollisions(rows, &rep); err != nil {
		return rep, err
	}
	if err := checkEEACodeCollisions(rows, &rep); err != nil {
		return rep, err
	}
	// The third writer into the id namespace, read before the transaction for
	// exactly that reason: only the rows that survive readEunisOnly's filters
	// reach the table, so only they can collide with a formation letter or a
	// primary code.
	byEEA := mapByEEACode(rows)
	eunisOnly, err := readEunisOnly(ctx, dir, byEEA, &rep)
	if err != nil {
		return SyntaxaReport{}, err
	}
	if err := checkIDNamespace(formations, rows, eunisOnly, &rep); err != nil {
		return rep, err
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return SyntaxaReport{}, fmt.Errorf("beginning syntaxa ingest transaction: %w", err)
	}
	if err := writeSyntaxa(ctx, tx, dir, syntaxaSources{
		formations: formations, rows: rows, byEEA: byEEA, eunisOnly: eunisOnly,
	}, &rep); err != nil {
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
	src syntaxaSources, rep *SyntaxaReport) error {
	if err := tx.ClearSyntaxa(); err != nil {
		return fmt.Errorf("clearing syntaxa before ingest: %w", err)
	}
	written := map[string]bool{}
	graph := newWrittenGraph()
	if err := writeFormations(tx, src.formations, written, graph, rep); err != nil {
		return err
	}
	if err := writeHierarchy(tx, src.rows, src.formations, written, graph, rep); err != nil {
		return err
	}
	// A class whose formation letter is unknown is skipped (SkippedRows,
	// above) but its own order and alliance rows are written regardless —
	// writeHierarchy has no reason to know a class was dropped elsewhere in
	// the same CSV. Anything that still points nowhere after every hierarchy
	// row is written is exactly what Orphans exists to catch: it is not
	// limited to the EEA-only remainder (Step 5, below).
	checkDanglingParents(src.rows, written, rep)
	checkCycles(src.rows, written, rep)

	// Step 3: the EEA units that FloraVeg does not carry. Every other
	// unit is already represented by its FloraVeg row — writing it a
	// second time would put the same syntaxon into the index under two
	// ids; readEunisOnly dropped those before the transaction opened.
	if err := writeEunisOnly(tx, src.eunisOnly, written, graph, rep); err != nil {
		return err
	}
	// Step 4: the edges. An EEA syntaxon id with a FloraVeg counterpart
	// gets resolved to the primary code; one without stays as it is.
	if err := writeLinks(ctx, tx, dir, src.byEEA, written, rep); err != nil {
		return err
	}
	// Step 5: the remaining EEA-only rows' parents — name match, then
	// sibling consensus, then orphan.
	parents, err := assignRemainingParents(tx, src.eunisOnly, src.rows, rep)
	if err != nil {
		return err
	}
	graph.setResolvedParents(src.eunisOnly, parents)
	checkParentRanks(graph, rep)

	// A chain that breaks at one point makes the orientation service
	// worthless at exactly that point — that must not be a warning one
	// skims past.
	if len(rep.Orphans) > 0 {
		sort.Strings(rep.Orphans)
		return fmt.Errorf("%d syntaxa have no parent after every step: %s",
			len(rep.Orphans), strings.Join(rep.Orphans, ", "))
	}
	// Same weight, one level finer: the chain exists but skips a rank.
	if len(rep.WrongRankParents) > 0 {
		return fmt.Errorf("%d syntaxa hang under a parent of the wrong rank: %s",
			len(rep.WrongRankParents), strings.Join(rep.WrongRankParents, "; "))
	}
	return nil
}

// mapEEACodeTo indexes the hierarchy rows by their eea_code, valueOf picking
// what the caller needs off the row: the FloraVeg primary code for the
// identity and edge remapping (mapByEEACode), the parent code for the EEA-only
// rows' parent derivation (assignRemainingParents).
//
// A plain index, with no ambiguity to resolve: checkEEACodeCollisions has
// already failed the ingest if two rows claimed one code, so the last write
// per key is also the only one.
func mapEEACodeTo(rows []hierarchyRow, valueOf func(hierarchyRow) string) map[string]string {
	byEEA := map[string]string{}
	for _, r := range rows {
		if r.eeaCode == "" {
			continue
		}
		byEEA[r.eeaCode] = valueOf(r)
	}
	return byEEA
}

// mapByEEACode maps each eea_code the hierarchy carries to its FloraVeg
// primary code.
func mapByEEACode(rows []hierarchyRow) map[string]string {
	return mapEEACodeTo(rows, func(r hierarchyRow) string { return r.code })
}

// writeFormations writes the 25 EuroVegChecklist sections as the root of
// the hierarchy: no parent, source evc, the life-form group from the file.
// Sorted by letter so a run is reproducible.
func writeFormations(tx output.IngestTx, formations map[string]domain.Syntaxon,
	written map[string]bool, graph *writtenGraph, rep *SyntaxaReport) error {
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
		graph.add(letter, domain.SyntaxonRankFormation, "")
		rep.FormationsWritten++
	}
	return nil
}

// writeHierarchy writes every FloraVeg class/order/alliance row. A class's
// parent is derived from its own code (formationOf); a class whose formation
// letter is not among the known formations is skipped and counted, never
// assigned a made-up parent. Order and alliance rows keep the parent_code
// the CSV already carries.
func writeHierarchy(tx output.IngestTx, rows []hierarchyRow, formations map[string]domain.Syntaxon,
	written map[string]bool, graph *writtenGraph, rep *SyntaxaReport) error {
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
		graph.add(r.code, r.rank, parentID)
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

// writeEunisOnly writes the rows readEunisOnly kept. ParentID stays empty;
// assignRemainingParents sets it by name and sibling consensus. Name is kept
// exactly as the file has it: the historical EUNIS combi-string with embedded
// authorship, never split heuristically.
func writeEunisOnly(tx output.IngestTx, eunisOnly []eunisOnlyRow,
	written map[string]bool, graph *writtenGraph, rep *SyntaxaReport) error {
	for _, r := range eunisOnly {
		s := domain.Syntaxon{
			ID:     r.id,
			Rank:   r.rank,
			Name:   r.name,
			Source: domain.SyntaxonSourceEUNIS,
		}
		if err := tx.UpsertSyntaxon(s); err != nil {
			return fmt.Errorf("upserting eunis-only %s: %w", r.id, err)
		}
		written[r.id] = true
		// Parent still empty here; assignRemainingParents decides it and
		// setResolvedParents folds it in afterwards.
		graph.add(r.id, r.rank, "")
		rep.EunisOnly++
	}
	return nil
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
