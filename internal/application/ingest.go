// Package application holds the use cases: ingest, localize, query. It
// depends only on internal/domain and internal/ports — never on an adapter.
package application

import (
	"context"
	"encoding/csv"
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

// IngestReport summarizes one ingest run.
type IngestReport struct {
	HabitatTypes  int
	Crosswalks    int
	Syntaxa       int
	SyntaxonLinks int
	SkippedRows   int
}

// IngestCSV loads typologies, habitat types, crosswalks and syntaxa from the
// CSVs in dir (produced by pipelines/eunis) into repo. It is one atomic
// transaction: any repository error rolls back and is returned; a malformed
// row is counted in SkippedRows and logged, never silently dropped and never
// aborting the run.
func IngestCSV(ctx context.Context, repo output.Repository, dir string) (IngestReport, error) {
	tx, err := repo.Begin(ctx)
	if err != nil {
		return IngestReport{}, fmt.Errorf("beginning ingest transaction: %w", err)
	}

	rep, err := ingestAll(tx, dir)
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

func ingestAll(tx output.IngestTx, dir string) (IngestReport, error) {
	var rep IngestReport

	if err := ingestTypologies(tx, dir); err != nil {
		return IngestReport{}, err
	}

	habitatTypes, skipped, err := ingestHabitatTypes(tx, dir)
	if err != nil {
		return IngestReport{}, err
	}
	rep.HabitatTypes = habitatTypes
	rep.SkippedRows += skipped

	crosswalks, skipped, err := ingestCrosswalks(tx, dir)
	if err != nil {
		return IngestReport{}, err
	}
	rep.Crosswalks = crosswalks
	rep.SkippedRows += skipped

	syntaxa, skipped, err := ingestSyntaxa(tx, dir)
	if err != nil {
		return IngestReport{}, err
	}
	rep.Syntaxa = syntaxa
	rep.SkippedRows += skipped

	links, skipped, err := ingestSyntaxonLinks(tx, dir)
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

func readAll(dir, name string, required []string, fn func(idx map[string]int, row []string, line int) error) error {
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
		if err == io.EOF {
			return nil
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

func ingestTypologies(tx output.IngestTx, dir string) error {
	const file = "typologies.csv"
	return readAll(dir, file, []string{"id", "scheme", "version", "name", "source_ref"},
		func(idx map[string]int, row []string, line int) error {
			id, err := domain.ParseTypologyID(row[idx["id"]])
			if err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
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
}

func ingestHabitatTypes(tx output.IngestTx, dir string) (count, skipped int, err error) {
	const file = "habitat_types.csv"
	err = readAll(dir, file,
		[]string{"typology_id", "code", "level", "name_en", "parent_code", "priority"},
		func(idx map[string]int, row []string, line int) error {
			typologyID, perr := domain.ParseTypologyID(row[idx["typology_id"]])
			if perr != nil {
				skipped++
				slog.Warn("skipping malformed habitat type row", "file", file, "line", line, "error", perr)
				return nil
			}
			level, perr := parseOptionalInt(row[idx["level"]])
			if perr != nil {
				skipped++
				slog.Warn("skipping malformed habitat type row", "file", file, "line", line, "error", perr)
				return nil
			}
			priority, perr := parseOptionalBool(row[idx["priority"]])
			if perr != nil {
				skipped++
				slog.Warn("skipping malformed habitat type row", "file", file, "line", line, "error", perr)
				return nil
			}
			h := domain.HabitatType{
				Key:        domain.HabitatTypeKey{Typology: typologyID, Code: row[idx["code"]]},
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

func ingestCrosswalks(tx output.IngestTx, dir string) (count, skipped int, err error) {
	const file = "crosswalks.csv"
	err = readAll(dir, file,
		[]string{"from_typology", "from_code", "to_typology", "to_code", "qualifier"},
		func(idx map[string]int, row []string, line int) error {
			fromTypology, perr := domain.ParseTypologyID(row[idx["from_typology"]])
			if perr != nil {
				skipped++
				slog.Warn("skipping malformed crosswalk row", "file", file, "line", line, "error", perr)
				return nil
			}
			toTypology, perr := domain.ParseTypologyID(row[idx["to_typology"]])
			if perr != nil {
				skipped++
				slog.Warn("skipping malformed crosswalk row", "file", file, "line", line, "error", perr)
				return nil
			}
			qualifier, perr := domain.ParseQualifier(row[idx["qualifier"]])
			if perr != nil {
				skipped++
				slog.Warn("skipping malformed crosswalk row", "file", file, "line", line, "error", perr)
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

func ingestSyntaxa(tx output.IngestTx, dir string) (count, skipped int, err error) {
	const file = "syntaxa.csv"
	err = readAll(dir, file, []string{"id", "rank", "name", "parent_id"},
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

func ingestSyntaxonLinks(tx output.IngestTx, dir string) (count, skipped int, err error) {
	const file = "habitat_type_syntaxa.csv"
	err = readAll(dir, file, []string{"typology_id", "code", "syntaxon_id"},
		func(idx map[string]int, row []string, line int) error {
			typologyID, perr := domain.ParseTypologyID(row[idx["typology_id"]])
			if perr != nil {
				skipped++
				slog.Warn("skipping malformed syntaxon link row", "file", file, "line", line, "error", perr)
				return nil
			}
			key := domain.HabitatTypeKey{Typology: typologyID, Code: row[idx["code"]]}
			if err := tx.LinkSyntaxon(key, row[idx["syntaxon_id"]]); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			count++
			return nil
		})
	return count, skipped, err
}
