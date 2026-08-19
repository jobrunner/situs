// Package application holds the use cases: ingest, localize, query. It
// depends only on internal/domain and internal/ports — never on an adapter.
package application

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// colCode/colTypologyID are the CSV column names shared by every file keyed
// on (typology_id, code): habitat types, syntaxon links and species roles.
const (
	colCode       = "code"
	colTypologyID = "typology_id"
)

// IngestReport summarizes one ingest run.
type IngestReport struct {
	HabitatTypes  int
	Crosswalks    int
	Syntaxa       int
	SyntaxonLinks int
	SkippedRows   int
}

// rowSkipper counts and logs a row this file could not use, without aborting
// the read. It is shared between a reader's per-value parse checks and
// readAll's own field-count check, so there is exactly one skip path per
// file instead of one per failure mode.
type rowSkipper func(line int, cause error)

func newRowSkipper(skipped *int, file, entity string) rowSkipper {
	return func(line int, cause error) {
		*skipped++
		slog.Warn("skipping malformed row", "entity", entity, "file", file, "line", line, "error", cause)
	}
}

// IngestCSV loads typologies, habitat types, crosswalks and syntaxa from the
// CSVs in dir (produced by pipelines/eunis) into repo. It is one atomic
// transaction: any repository error rolls back and is returned; a malformed
// row is counted in SkippedRows and logged, never silently dropped and never
// aborting the run (the sole exception is typologies.csv, see
// ingestTypologies).
func IngestCSV(ctx context.Context, repo output.Repository, dir string) (IngestReport, error) {
	tx, err := repo.Begin(ctx)
	if err != nil {
		return IngestReport{}, fmt.Errorf("beginning ingest transaction: %w", err)
	}

	rep, err := ingestAll(ctx, tx, dir)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return IngestReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return IngestReport{}, err
	}

	if err := tx.Commit(); err != nil {
		return IngestReport{}, fmt.Errorf("committing ingest transaction: %w", err)
	}
	return rep, nil
}

func ingestAll(ctx context.Context, tx output.IngestTx, dir string) (IngestReport, error) {
	var rep IngestReport

	skippedTypologies, err := ingestTypologies(ctx, tx, dir)
	if err != nil {
		return IngestReport{}, err
	}
	rep.SkippedRows += skippedTypologies

	habitatTypes, skipped, err := ingestHabitatTypes(ctx, tx, dir)
	if err != nil {
		return IngestReport{}, err
	}
	rep.HabitatTypes = habitatTypes
	rep.SkippedRows += skipped

	crosswalks, skipped, err := ingestCrosswalks(ctx, tx, dir)
	if err != nil {
		return IngestReport{}, err
	}
	rep.Crosswalks = crosswalks
	rep.SkippedRows += skipped

	syntaxa, skipped, err := ingestSyntaxa(ctx, tx, dir)
	if err != nil {
		return IngestReport{}, err
	}
	rep.Syntaxa = syntaxa
	rep.SkippedRows += skipped

	links, skipped, err := ingestSyntaxonLinks(ctx, tx, dir)
	if err != nil {
		return IngestReport{}, err
	}
	rep.SyntaxonLinks = links
	rep.SkippedRows += skipped

	return rep, nil
}

// csvReader opens name in dir and returns its rows plus a header index by
// column name. Columns are looked up by name, never by position, so the
// pipeline may add columns without breaking the ingest. A required column
// missing from the header aborts this file's ingest.
//
// os.OpenRoot confines the open to dir: name can never escape it via "..",
// so this stays safe even though dir is a caller-supplied path.
func csvReader(dir, name string) (*csv.Reader, *os.File, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("opening ingest directory %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }() // the returned *os.File keeps its own fd

	f, err := root.Open(name)
	if err != nil {
		return nil, nil, fmt.Errorf("opening %s: %w", filepath.Join(dir, name), err)
	}
	return csv.NewReader(f), f, nil
}

func headerIndex(header []string, required ...string) (map[string]int, error) {
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[strings.TrimSpace(name)] = i
	}
	for _, name := range required {
		if _, ok := idx[name]; !ok {
			return nil, fmt.Errorf("missing required column %q", name)
		}
	}
	return idx, nil
}

// readAll drives one CSV file: header lookup, then one fn call per data row.
// A row with the wrong number of fields (csv.ErrFieldCount — the shape a
// truncated or hand-edited row takes) is reported through skip and the read
// continues; every other reader error (unterminated quote, I/O) aborts, as
// does an fn error.
func readAll(ctx context.Context, dir, name string, required []string, skip rowSkipper,
	fn func(idx map[string]int, row []string, line int) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ingesting %s: %w", name, err)
	}

	r, f, err := csvReader(dir, name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("reading header of %s: %w", name, err)
	}
	idx, err := headerIndex(header, required...)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	line := 1
	for {
		row, err := r.Read()
		line++
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, csv.ErrFieldCount) {
			skip(line, err)
			continue
		}
		if err != nil {
			return fmt.Errorf("reading %s at line %d: %w", name, line, err)
		}
		if err := fn(idx, row, line); err != nil {
			return err
		}
	}
}

func parseOptionalInt(s string) (*int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func parseOptionalBool(s string) (*bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	b := n != 0
	return &b, nil
}

// ingestTypologies is the one exception to "a malformed row is skipped, not
// aborting": a typology is the join target for every habitat type,
// crosswalk and syntaxon link. Skipping past an unparseable typology row
// would let those rows reference a typology that was never inserted,
// silently building an index with dangling references — worse than stopping
// the ingest. A wrong field count (readAll's own check) still only counts as
// a skip, matching every other file.
func ingestTypologies(ctx context.Context, tx output.IngestTx, dir string) (skipped int, err error) {
	const file = "typologies.csv"
	skip := newRowSkipper(&skipped, file, "typology")
	err = readAll(ctx, dir, file, []string{"id", "scheme", "version", "name", "source_ref"}, skip,
		func(idx map[string]int, row []string, line int) error {
			id, perr := domain.ParseTypologyID(row[idx["id"]])
			if perr != nil {
				return fmt.Errorf("%s:%d: %w", file, line, perr)
			}
			t := domain.Typology{
				ID:        id,
				Scheme:    row[idx["scheme"]],
				Version:   row[idx["version"]],
				Name:      row[idx["name"]],
				SourceRef: row[idx["source_ref"]],
			}
			if err := tx.UpsertTypology(t); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			return nil
		})
	return skipped, err
}

func ingestHabitatTypes(ctx context.Context, tx output.IngestTx, dir string) (count, skipped int, err error) {
	const file = "habitat_types.csv"
	skip := newRowSkipper(&skipped, file, "habitat type")
	err = readAll(ctx, dir, file,
		[]string{colTypologyID, colCode, "level", "name_en", "parent_code", "priority"}, skip,
		func(idx map[string]int, row []string, line int) error {
			typologyID, perr := domain.ParseTypologyID(row[idx[colTypologyID]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			level, perr := parseOptionalInt(row[idx["level"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			priority, perr := parseOptionalBool(row[idx["priority"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			h := domain.HabitatType{
				Key:        domain.HabitatTypeKey{Typology: typologyID, Code: row[idx[colCode]]},
				Level:      level,
				NameEN:     row[idx["name_en"]],
				ParentCode: row[idx["parent_code"]],
				Priority:   priority,
			}
			if err := tx.UpsertHabitatType(h); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			count++
			return nil
		})
	return count, skipped, err
}

func ingestCrosswalks(ctx context.Context, tx output.IngestTx, dir string) (count, skipped int, err error) {
	const file = "crosswalks.csv"
	skip := newRowSkipper(&skipped, file, "crosswalk")
	err = readAll(ctx, dir, file,
		[]string{"from_typology", "from_code", "to_typology", "to_code", "qualifier"}, skip,
		func(idx map[string]int, row []string, line int) error {
			fromTypology, perr := domain.ParseTypologyID(row[idx["from_typology"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			toTypology, perr := domain.ParseTypologyID(row[idx["to_typology"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			qualifier, perr := domain.ParseQualifier(row[idx["qualifier"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			c := domain.Crosswalk{
				From:      domain.HabitatTypeKey{Typology: fromTypology, Code: row[idx["from_code"]]},
				To:        domain.HabitatTypeKey{Typology: toTypology, Code: row[idx["to_code"]]},
				Qualifier: qualifier,
			}
			if err := tx.UpsertCrosswalk(c); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			count++
			return nil
		})
	return count, skipped, err
}

func ingestSyntaxa(ctx context.Context, tx output.IngestTx, dir string) (count, skipped int, err error) {
	const file = "syntaxa.csv"
	skip := newRowSkipper(&skipped, file, "syntaxon")
	err = readAll(ctx, dir, file, []string{"id", "rank", "name", "parent_id"}, skip,
		func(idx map[string]int, row []string, line int) error {
			s := domain.Syntaxon{
				ID:       row[idx["id"]],
				Rank:     row[idx["rank"]],
				Name:     row[idx["name"]],
				ParentID: row[idx["parent_id"]],
			}
			if err := tx.UpsertSyntaxon(s); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			count++
			return nil
		})
	return count, skipped, err
}

// SpeciesReport summarizes one species-role ingest run.
type SpeciesReport struct {
	Rows       int
	Resolved   int
	Unresolved int
}

// ResolutionRate is the fraction of rows whose verbatim name resolved to a
// hostus concept ID, measured against the total row count (not the distinct
// name count) — the same population the design spec's open point 3 asks for.
func (r SpeciesReport) ResolutionRate() float64 {
	if r.Rows == 0 {
		return 0
	}
	return float64(r.Resolved) / float64(r.Rows)
}

// parseOptionalFloat mirrors parseOptionalInt: an empty fidelity/constancy
// column is absence of data, never a zero value.
func parseOptionalFloat(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	return &f, nil
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

// distinctNames returns the deduplicated verbatim names across rows, so
// Resolve is called once for the set, not once per row.
func distinctNames(rows []speciesRow) []string {
	names := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		names[r.verbatim] = struct{}{}
	}
	distinct := make([]string, 0, len(names))
	for n := range names {
		distinct = append(distinct, n)
	}
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

	if err := tx.Commit(); err != nil {
		return SpeciesReport{}, fmt.Errorf("committing species-role ingest transaction: %w", err)
	}
	return rep, nil
}

func ingestSyntaxonLinks(ctx context.Context, tx output.IngestTx, dir string) (count, skipped int, err error) {
	const file = "habitat_type_syntaxa.csv"
	skip := newRowSkipper(&skipped, file, "syntaxon link")
	err = readAll(ctx, dir, file, []string{colTypologyID, colCode, "syntaxon_id"}, skip,
		func(idx map[string]int, row []string, line int) error {
			typologyID, perr := domain.ParseTypologyID(row[idx[colTypologyID]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			key := domain.HabitatTypeKey{Typology: typologyID, Code: row[idx[colCode]]}
			if err := tx.LinkSyntaxon(key, row[idx["syntaxon_id"]]); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			count++
			return nil
		})
	return count, skipped, err
}
