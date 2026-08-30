package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// SpeciesReport summarizes one species-role ingest run. Skipped mirrors
// IngestReport.SkippedRows — the same JSON object situs ingest prints
// carries both reports, and a row dropped from one report but invisible in
// the other is a trap for the operator reading that output.
type SpeciesReport struct {
	Rows       int
	Resolved   int
	Unresolved int
	Skipped    int
	// DerivedRows is the number of aggregate-member rows this run added.
	DerivedRows int
	// SuppressedByExplicit counts derived rows NOT written because their key
	// was already occupied — usually by an explicit species_roles.csv row
	// (which always wins), but the same INSERT ... ON CONFLICT DO NOTHING
	// also fires for a duplicate aggregate_members.csv row or two aggregates
	// naming the same member: the index cannot distinguish which case it
	// was after the fact, so this counts "derivation lost the race", not
	// specifically "an explicit row was the cause". Not a defect either way.
	SuppressedByExplicit int
	// AmbiguousCrosswalk counts verbatim names with more than one distinct
	// concept id in eurosl_crosswalk.csv — never guessed, kept unresolved.
	AmbiguousCrosswalk int
}

// ResolutionRate is the fraction of rows whose verbatim name resolved to a
// concept ID, measured against the total row count (not the distinct name
// count) — the same population the design spec's open point 3 asks for.
// This is row-weighted, a different population from the ESy spike's ~57%
// distinct-name floor; the two are not directly comparable — see
// docs/how-to/ingest.md.
func (r SpeciesReport) ResolutionRate() float64 {
	if r.Rows == 0 {
		return 0
	}
	return float64(r.Resolved) / float64(r.Rows)
}

const (
	speciesProvenanceObserved             = "observed"
	speciesProvenanceDerivedFromAggregate = "derived_from_aggregate"
)

// speciesRow is one parsed species_roles.csv row.
type speciesRow struct {
	key       domain.HabitatTypeKey
	verbatim  string
	role      string
	fidelity  *float64
	constancy *float64
}

// readSpeciesRows parses dir/file (species_roles.csv), skipping malformed
// rows the same way every other ingest file does.
func readSpeciesRows(ctx context.Context, dir, file string, skip rowSkipper) ([]speciesRow, error) {
	var rows []speciesRow
	err := readAll(ctx, dir, file, ',',
		[]string{colTypologyID, colCode, "verbatim_name", "role", "fidelity", "constancy"}, skip,
		func(idx map[string]int, r []string, line int) error {
			typologyID, perr := domain.ParseTypologyID(r[idx[colTypologyID]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			fidelity, perr := parseOptionalFloat(r[idx["fidelity"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			constancy, perr := parseOptionalFloat(r[idx["constancy"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			rows = append(rows, speciesRow{
				key:       domain.HabitatTypeKey{Typology: typologyID, Code: r[idx[colCode]]},
				verbatim:  r[idx["verbatim_name"]],
				role:      r[idx["role"]],
				fidelity:  fidelity,
				constancy: constancy,
			})
			return nil
		})
	return rows, err
}

// tallyResolution classifies one row's resolution outcome into rep and
// returns the concept id to store — nil unless the row resolved cleanly.
// Two independent guard clauses, not an if-else chain: each is its own
// mutation-coverable branch, unlike a three-way switch/if-else-if, whose
// case-expression negation gremlins cannot distinguish from adjacent cases.
func tallyResolution(verbatim, id string, ambiguous bool, rep *SpeciesReport) *string {
	if ambiguous {
		rep.AmbiguousCrosswalk++
		rep.Unresolved++
		slog.Warn("ambiguous crosswalk entry: more than one concept id, name stays unresolved",
			"verbatim_name", verbatim)
		return nil
	}
	if id == "" {
		rep.Unresolved++
		return nil
	}
	rep.Resolved++
	return &id
}

// resolveRow looks verbatim up in crosswalk. More than one DISTINCT concept
// id for the same name is a data find, not a guessing occasion: ambiguous is
// true and conceptID stays empty. Repeated identical rows are not ambiguous.
func resolveRow(verbatim string, crosswalk map[string][]string) (conceptID string, ambiguous bool) {
	ids, ok := crosswalk[verbatim]
	if !ok || len(ids) == 0 {
		return "", false
	}
	distinct := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		distinct[id] = struct{}{}
	}
	if len(distinct) > 1 {
		return "", true
	}
	return ids[0], false
}

// upsertSpeciesRows writes every explicit row first, then attempts aggregate
// derivation — in that fixed order, never interleaved, so an explicit row
// always exists (or does not) before its key is ever contested by a derived
// one. That is what makes "explicit always wins" independent of csv row
// order: derivation only ever sees a fully-written explicit layer.
func upsertSpeciesRows(tx output.IngestTx, rows []speciesRow, crosswalk map[string][]string,
	aggregates map[string][]aggregateMember) (SpeciesReport, error) {
	rep := SpeciesReport{Rows: len(rows)}

	for _, r := range rows {
		if err := upsertExplicitRow(tx, r, crosswalk, &rep); err != nil {
			return SpeciesReport{}, err
		}
	}
	for _, r := range rows {
		if err := deriveAggregateMembers(tx, r, crosswalk, aggregates, &rep); err != nil {
			return SpeciesReport{}, err
		}
	}
	return rep, nil
}

// upsertExplicitRow writes one species_roles.csv row as-is, tallying its
// resolution outcome into rep.
func upsertExplicitRow(tx output.IngestTx, r speciesRow, crosswalk map[string][]string, rep *SpeciesReport) error {
	id, ambiguous := resolveRow(r.verbatim, crosswalk)
	conceptID := tallyResolution(r.verbatim, id, ambiguous, rep)
	return tx.UpsertSpeciesRole(domain.SpeciesRole{
		Key:          r.key,
		ConceptID:    conceptID,
		VerbatimName: r.verbatim,
		Role:         r.role,
		Fidelity:     r.fidelity,
		Constancy:    r.constancy,
		Provenance:   speciesProvenanceObserved,
	})
}

// deriveAggregateMembers writes one derived row per member species, if r's
// resolved concept id is itself a listed aggregate — a no-op otherwise.
func deriveAggregateMembers(tx output.IngestTx, r speciesRow, crosswalk map[string][]string,
	aggregates map[string][]aggregateMember, rep *SpeciesReport) error {
	id, ambiguous := resolveRow(r.verbatim, crosswalk)
	if ambiguous || id == "" {
		return nil
	}
	members, isAggregate := aggregates[id]
	if !isAggregate {
		return nil
	}
	aggregateID := id
	for _, m := range members {
		memberID := m.conceptID
		suppressed, err := tx.UpsertDerivedSpeciesRole(domain.SpeciesRole{
			Key:          r.key,
			ConceptID:    &memberID,
			VerbatimName: m.name,
			Role:         r.role,
			Provenance:   speciesProvenanceDerivedFromAggregate,
			DerivedFrom:  &aggregateID,
		})
		if err != nil {
			return err
		}
		if suppressed {
			rep.SuppressedByExplicit++
		} else {
			rep.DerivedRows++
		}
	}
	return nil
}

// IngestSpeciesRoles loads csvPath (species_roles.csv, produced by
// pipelines/eunis) into repo, resolving every row's verbatim name against
// the local crosswalkPath dictionary — no network call, no hostus dependency
// in this step. For every row whose resolved concept id is itself an
// aggregate listed in aggregateMembersPath, it additionally writes one
// derived row per member species, unless an explicit row already occupies
// that member's key. A missing crosswalkPath aborts the ingest (nothing can
// resolve without it); a missing aggregateMembersPath does not (derivation
// is extra information).
func IngestSpeciesRoles(ctx context.Context, repo output.Repository, csvPath, crosswalkPath,
	aggregateMembersPath string) (SpeciesReport, error) {
	dir, file := splitCSVPath(csvPath)

	skipped := 0
	rows, err := readSpeciesRows(ctx, dir, file, newRowSkipper(&skipped, file, "species role"))
	if err != nil {
		return SpeciesReport{}, err
	}

	crosswalk, crosswalkSkipped, err := loadCrosswalk(ctx, crosswalkPath)
	if err != nil {
		return SpeciesReport{}, fmt.Errorf("loading crosswalk %q: %w", crosswalkPath, err)
	}
	skipped += crosswalkSkipped

	aggregates, aggregatesSkipped, err := loadAggregateMembers(ctx, aggregateMembersPath)
	if err != nil {
		return SpeciesReport{}, fmt.Errorf("loading aggregate members %q: %w", aggregateMembersPath, err)
	}
	skipped += aggregatesSkipped

	tx, err := repo.Begin(ctx)
	if err != nil {
		return SpeciesReport{}, fmt.Errorf("beginning species-role ingest transaction: %w", err)
	}

	rep, err := upsertSpeciesRows(tx, rows, crosswalk, aggregates)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return SpeciesReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return SpeciesReport{}, err
	}
	rep.Skipped = skipped

	if err := tx.Commit(); err != nil {
		return SpeciesReport{}, fmt.Errorf("committing species-role ingest transaction: %w", err)
	}
	return rep, nil
}
