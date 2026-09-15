package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// AreaReport summarizes the WGSRPD area-name ingest.
type AreaReport struct {
	Areas       int
	SkippedRows int
}

// IngestAreas loads csvPath (wgsrpd_areas.csv: area_scheme,area_code,name_en,
// produced by pipelines/wgsrpd) into repo, in one transaction.
//
// It reads a local CSV and nothing else — the area names are exactly the kind
// of static reference data situs must not need another service for.
//
// A missing csvPath is "no area names pinned yet", not an error, mirroring
// IngestSyntaxaHierarchy: an index without them still answers every query,
// its area codes simply stay unnamed.
func IngestAreas(ctx context.Context, repo output.Repository, csvPath string) (AreaReport, error) {
	if _, err := os.Stat(csvPath); err != nil {
		if os.IsNotExist(err) {
			slog.InfoContext(ctx, "no area name file, skipping", "path", csvPath)
			return AreaReport{}, nil
		}
		return AreaReport{}, fmt.Errorf("statting %s: %w", csvPath, err)
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return AreaReport{}, fmt.Errorf("beginning area ingest transaction: %w", err)
	}

	rep, err := ingestAreaRows(ctx, tx, csvPath)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return AreaReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return AreaReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return AreaReport{}, fmt.Errorf("committing area ingest transaction: %w", err)
	}
	if rep.SkippedRows > 0 {
		slog.WarnContext(ctx, "skipped malformed rows in the area name file",
			"path", csvPath, "skipped", rep.SkippedRows)
	}
	return rep, nil
}

// ingestAreaRows parses and writes the rows. A row whose area is incomplete or
// names a foreign scheme is skipped and counted: it could never match a
// distribution row. An empty name is not a defect — the code is the fact, the
// name the overlay.
func ingestAreaRows(ctx context.Context, tx output.IngestTx, csvPath string) (AreaReport, error) {
	dir, file := filepath.Split(csvPath)
	var rep AreaReport
	skip := newRowSkipper(&rep.SkippedRows, file, "area")

	err := readAll(ctx, dir, file, ',', []string{"area_scheme", "area_code", "name_en"}, skip,
		func(idx map[string]int, row []string, line int) error {
			a := domain.NamedArea{
				Area: domain.Area{
					Scheme: row[idx["area_scheme"]],
					Code:   row[idx["area_code"]],
				},
				NameEN: row[idx["name_en"]],
			}
			if !a.IsComplete() {
				skip(line, fmt.Errorf("incomplete area %s", a.Area))
				return nil
			}
			// situs stores exactly one area scheme. A row naming another one
			// would be written and counted as a success, yet join with nothing
			// on the read side — that silence is the whole reason to reject it
			// here, where the report can show it.
			if a.Scheme != domain.SchemeWGSRPDL3 {
				skip(line, fmt.Errorf("area scheme %q is not %q", a.Scheme, domain.SchemeWGSRPDL3))
				return nil
			}
			if err := tx.UpsertArea(a); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			rep.Areas++
			return nil
		})
	if err != nil {
		return AreaReport{}, err
	}
	return rep, nil
}
