package application

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

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
}

// ResolutionRate is the fraction of rows whose verbatim name resolved to a
// hostus concept ID, measured against the total row count (not the distinct
// name count) — the same population the design spec's open point 3 asks
// for. This is row-weighted, a different population from the ESy spike's
// ~57% distinct-name floor; the two are not directly comparable — see
// docs/how-to/ingest.md.
func (r SpeciesReport) ResolutionRate() float64 {
	if r.Rows == 0 {
		return 0
	}
	return float64(r.Resolved) / float64(r.Rows)
}

// speciesRow is one parsed species_roles.csv row, held in memory only long
// enough to collect the distinct verbatim names before the single Resolve
// call and the subsequent upserts.
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
	err := readAll(ctx, dir, file,
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

// distinctNames returns the deduplicated verbatim names across rows, sorted
// so batch composition (and thus which names land in which /v1/match
// request) is reproducible run to run, not dependent on map iteration
// order — a hostus-side discrepancy must be reproducible to be debuggable.
func distinctNames(rows []speciesRow) []string {
	names := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		names[r.verbatim] = struct{}{}
	}
	distinct := make([]string, 0, len(names))
	for n := range names {
		distinct = append(distinct, n)
	}
	sort.Strings(distinct)
	return distinct
}

// upsertSpeciesRows stores every row via tx, setting ConceptID only where
// resolved has an entry for its verbatim name, and tallies rep accordingly.
func upsertSpeciesRows(tx output.IngestTx, rows []speciesRow, resolved map[string]string) (SpeciesReport, error) {
	rep := SpeciesReport{Rows: len(rows)}
	for _, r := range rows {
		var conceptID *string
		if id, ok := resolved[r.verbatim]; ok {
			conceptID = &id
			rep.Resolved++
		} else {
			rep.Unresolved++
		}
		sr := domain.SpeciesRole{
			Key:          r.key,
			ConceptID:    conceptID,
			VerbatimName: r.verbatim,
			Role:         r.role,
			Fidelity:     r.fidelity,
			Constancy:    r.constancy,
		}
		if err := tx.UpsertSpeciesRole(sr); err != nil {
			return SpeciesReport{}, err
		}
	}
	return rep, nil
}

// IngestSpeciesRoles loads csvPath (species_roles.csv, produced by
// pipelines/eunis) into repo, crosswalking every distinct verbatim name to a
// hostus concept ID in one Resolve call — the file's ~13800 rows carry far
// fewer distinct names, and hostus is a network hop. A resolver error aborts
// the ingest; it must never be recorded as "every name unresolvable". An
// unresolved name is still stored, with ConceptID left nil.
func IngestSpeciesRoles(ctx context.Context, repo output.Repository, resolver output.NameResolver,
	csvPath string) (SpeciesReport, error) {
	dir, file := filepath.Split(csvPath)

	skipped := 0
	rows, err := readSpeciesRows(ctx, dir, file, newRowSkipper(&skipped, file, "species role"))
	if err != nil {
		return SpeciesReport{}, err
	}

	resolved, err := resolver.Resolve(ctx, distinctNames(rows))
	if err != nil {
		return SpeciesReport{}, fmt.Errorf("resolving species names via hostus: %w", err)
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return SpeciesReport{}, fmt.Errorf("beginning species-role ingest transaction: %w", err)
	}

	rep, err := upsertSpeciesRows(tx, rows, resolved)
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
