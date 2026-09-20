package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// DescriptionReport summarizes the factsheet-description ingest.
type DescriptionReport struct {
	Written     int
	SkippedRows int
	// SkippedUnknownCode counts descriptions whose habitat type this index
	// does not carry. The factsheets cover habitats the EEA classification in
	// this index does not (the marine MA* codes, N23, N24): storing those
	// would put rows where no route can reach them.
	SkippedUnknownCode int
}

// IngestDescriptions loads csvPath (habitat_descriptions.csv:
// typology_id,code,description_en, produced by pipelines/floraveg-factsheets)
// into repo, in one transaction. source names the artifact behind the text and
// is stored with every row.
//
// A missing csvPath is "no factsheets pinned yet", not an error, mirroring the
// other optional ingest steps.
func IngestDescriptions(ctx context.Context, repo output.Repository, csvPath, source string) (DescriptionReport, error) {
	if _, err := os.Stat(csvPath); err != nil {
		if os.IsNotExist(err) {
			slog.InfoContext(ctx, "no habitat description file, skipping", "path", csvPath)
			return DescriptionReport{}, nil
		}
		return DescriptionReport{}, fmt.Errorf("statting %s: %w", csvPath, err)
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return DescriptionReport{}, fmt.Errorf("beginning description ingest transaction: %w", err)
	}

	rep, err := ingestDescriptionRows(ctx, repo, tx, csvPath, source)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return DescriptionReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return DescriptionReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return DescriptionReport{}, fmt.Errorf("committing description ingest transaction: %w", err)
	}

	if rep.SkippedRows > 0 {
		slog.WarnContext(ctx, "skipped malformed rows in the description file",
			"path", csvPath, "skipped", rep.SkippedRows)
	}
	if rep.SkippedUnknownCode > 0 {
		slog.InfoContext(ctx, "dropped descriptions for habitat types this index does not carry",
			"path", csvPath, "dropped", rep.SkippedUnknownCode)
	}
	return rep, nil
}

// ingestDescriptionRows parses and writes the rows. Reading through repo (not
// tx) to check the habitat type is deliberate: the check asks what the index
// already holds, and the descriptions are written in their own transaction
// after every other entity is committed.
func ingestDescriptionRows(ctx context.Context, repo output.Repository, tx output.IngestTx,
	csvPath, source string,
) (DescriptionReport, error) {
	dir, file := splitCSVPath(csvPath)
	var rep DescriptionReport
	skip := newRowSkipper(&rep.SkippedRows, file, "habitat description")

	err := readAll(ctx, dir, file, ',', []string{"typology_id", "code", "description_en"}, skip,
		func(idx map[string]int, row []string, line int) error {
			typology, perr := domain.ParseTypologyID(row[idx["typology_id"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			key := domain.HabitatTypeKey{Typology: typology, Code: row[idx["code"]]}
			text := row[idx["description_en"]]
			if key.Code == "" || text == "" {
				skip(line, fmt.Errorf("incomplete description row for %q", key.Code))
				return nil
			}
			if _, err := repo.HabitatType(ctx, key); err != nil {
				if errors.Is(err, output.ErrNotFound) {
					rep.SkippedUnknownCode++
					return nil
				}
				return fmt.Errorf("%s:%d: looking up %s: %w", file, line, key, err)
			}
			if err := tx.UpsertDescription(domain.HabitatDescription{
				Key: key, TextEN: text, Source: source,
			}); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			rep.Written++
			return nil
		})
	if err != nil {
		return DescriptionReport{}, err
	}
	return rep, nil
}
