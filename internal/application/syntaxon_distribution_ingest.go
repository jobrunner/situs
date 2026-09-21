package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

const (
	fileSyntaxonDistribution         = "syntaxon_distribution.csv"
	fileSyntaxonDistributionCoverage = "syntaxon_distribution_coverage.csv"

	// colSyntaxonID/colAreaScheme/colAreaCode/colOccurrence name the columns
	// both distribution CSVs share with each other and (the first two) with
	// area_ingest.go's files. Named once here so goconst does not count the
	// same literal across this file's two readers as a near-duplicate.
	colSyntaxonID = "syntaxon_id"
	colAreaScheme = "area_scheme"
	colAreaCode   = "area_code"
	colOccurrence = "occurrence"
)

// SyntaxonDistributionReport counts what a distribution ingest wrote and what
// it could not place. Every number is counted, none is estimated.
type SyntaxonDistributionReport struct {
	Written   int
	Verified  int
	Uncertain int
	Covered   int

	SkippedRows int

	// UnknownSyntaxa are source codes the index does not know, each listed
	// once however many rows it had, sorted. Measured against the pinned
	// artifacts this is exactly one entry, CI01E: it is in the distribution
	// file (EVC fassung 3, 2024-06-12) but not in the pinned FloraVeg file.
	// Real fassung drift between two sources, not a defect of this ingest —
	// and naming it is the point.
	UnknownSyntaxa []string
}

// IngestSyntaxonDistribution loads the two CSVs pipelines/evc-distribution
// produces into repo, in one transaction.
//
// A missing csvPath is "no syntaxon distribution pinned yet", not an error:
// the distribution is extra information, exactly like IngestDistribution's,
// and a five-minute ingest must not die over an optional source. The coverage
// file is the one exception — see below.
//
// Runs AFTER IngestSyntaxa: the syntaxon ids have to be in the index for
// UnknownSyntaxa to mean anything. No hostus involvement — this is a pure CSV
// source, and the ingest stays offline for it.
func IngestSyntaxonDistribution(ctx context.Context, repo output.Repository, csvPath, coveragePath string) (SyntaxonDistributionReport, error) {
	skip, err := checkDistributionFiles(ctx, csvPath, coveragePath)
	if err != nil {
		return SyntaxonDistributionReport{}, err
	}
	if skip {
		return SyntaxonDistributionReport{}, nil
	}

	known, err := knownSyntaxonIDs(ctx, repo)
	if err != nil {
		return SyntaxonDistributionReport{}, err
	}

	rep, err := ingestSyntaxonDistributionTx(ctx, repo, csvPath, coveragePath, known)
	if err != nil {
		return SyntaxonDistributionReport{}, err
	}

	if len(rep.UnknownSyntaxa) > 0 {
		slog.WarnContext(ctx, "the distribution source names syntaxa the index does not carry",
			"count", len(rep.UnknownSyntaxa), "codes", rep.UnknownSyntaxa)
	}
	if rep.SkippedRows > 0 {
		slog.WarnContext(ctx, "skipped malformed rows in the syntaxon distribution files",
			"path", csvPath, "skipped", rep.SkippedRows)
	}
	return rep, nil
}

// checkDistributionFiles reports whether the whole ingest should be skipped
// (csvPath simply does not exist yet — extra information, not a defect) or
// must fail (any other Stat error, or the distribution present without its
// coverage file). The distribution WITHOUT the coverage is the one
// combination that must not pass: every occurrence row would be written, and
// every alliance's empty cell would then read as "nobody looked" instead of
// "does not occur" — the four states collapse to three, silently, for the
// whole index. Half of this source is worse than none of it.
func checkDistributionFiles(ctx context.Context, csvPath, coveragePath string) (skip bool, err error) {
	if _, statErr := os.Stat(csvPath); statErr != nil {
		if os.IsNotExist(statErr) {
			slog.InfoContext(ctx, "no syntaxon distribution file, skipping", "path", csvPath)
			return true, nil
		}
		return false, fmt.Errorf("statting %s: %w", csvPath, statErr)
	}
	if _, statErr := os.Stat(coveragePath); statErr != nil {
		return false, fmt.Errorf(
			"%s is present but %s is not; without the coverage rows an absence cannot be told from an unknown: %w",
			fileSyntaxonDistribution, fileSyntaxonDistributionCoverage, statErr)
	}
	return false, nil
}

func knownSyntaxonIDs(ctx context.Context, repo output.Repository) (map[string]bool, error) {
	all, err := repo.AllSyntaxa(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing syntaxa: %w", err)
	}
	known := make(map[string]bool, len(all))
	for _, s := range all {
		known[s.ID] = true
	}
	return known, nil
}

// ingestSyntaxonDistributionTx owns the transaction lifecycle: begin, write,
// rollback-or-commit. Split out of IngestSyntaxonDistribution to keep that
// function's cyclomatic complexity under the package's ratchet.
func ingestSyntaxonDistributionTx(ctx context.Context, repo output.Repository, csvPath, coveragePath string, known map[string]bool) (SyntaxonDistributionReport, error) {
	tx, err := repo.Begin(ctx)
	if err != nil {
		return SyntaxonDistributionReport{}, fmt.Errorf("beginning syntaxon distribution transaction: %w", err)
	}
	rep, err := writeSyntaxonDistribution(ctx, tx, csvPath, coveragePath, known)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return SyntaxonDistributionReport{}, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return SyntaxonDistributionReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntaxonDistributionReport{}, fmt.Errorf("committing syntaxon distribution transaction: %w", err)
	}
	return rep, nil
}

// unknownTracker records an id once, however many rows it has. One unknown
// alliance can carry up to 136 rows, and reporting it 136 times would turn
// one fassung drift into a wall of noise.
type unknownTracker struct {
	seen map[string]bool
	ids  []string
}

func (u *unknownTracker) add(id string) {
	if u.seen[id] {
		return
	}
	u.seen[id] = true
	u.ids = append(u.ids, id)
}

func writeSyntaxonDistribution(ctx context.Context, tx output.IngestTx, csvPath, coveragePath string, known map[string]bool) (SyntaxonDistributionReport, error) {
	var rep SyntaxonDistributionReport
	unknown := &unknownTracker{seen: map[string]bool{}}

	if err := readOccurrenceRows(ctx, tx, csvPath, known, unknown, &rep); err != nil {
		return SyntaxonDistributionReport{}, err
	}
	if err := readCoverageRows(ctx, tx, coveragePath, known, unknown, &rep); err != nil {
		return SyntaxonDistributionReport{}, err
	}

	slices.Sort(unknown.ids)
	rep.UnknownSyntaxa = unknown.ids
	return rep, nil
}

func readOccurrenceRows(ctx context.Context, tx output.IngestTx, csvPath string,
	known map[string]bool, unknown *unknownTracker, rep *SyntaxonDistributionReport) error {
	// splitCSVPath, not filepath.Split: a bare relative filename splits to
	// dir="", which os.OpenRoot does not read as the current directory.
	dir, file := splitCSVPath(csvPath)
	skip := newRowSkipper(&rep.SkippedRows, file, "syntaxon distribution")

	return readAll(ctx, dir, file, ',',
		[]string{colSyntaxonID, colAreaScheme, colAreaCode, colOccurrence}, skip,
		func(idx map[string]int, row []string, line int) error {
			id := row[idx[colSyntaxonID]]
			a := domain.Area{Scheme: row[idx[colAreaScheme]], Code: row[idx[colAreaCode]]}
			occurrence := row[idx[colOccurrence]]

			if id == "" || !a.IsComplete() {
				skip(line, fmt.Errorf("incomplete row: syntaxon %q, area %s", id, a))
				return nil
			}
			if !domain.IsKnownAreaScheme(a.Scheme) {
				skip(line, fmt.Errorf("area scheme %q is not one of %v", a.Scheme, domain.KnownAreaSchemes()))
				return nil
			}
			// The pipeline aborts on an unknown cell value, so this row should
			// not exist. Skipped rather than written because the CHECK would
			// reject it and a failed transaction would lose every other row.
			if occurrence != domain.OccurrenceVerified && occurrence != domain.OccurrenceUncertain {
				skip(line, fmt.Errorf("occurrence %q is neither %q nor %q",
					occurrence, domain.OccurrenceVerified, domain.OccurrenceUncertain))
				return nil
			}
			// A code the index does not know is NOT written: a distribution row
			// pointing at nothing would answer no question and would make
			// AreasWithData offer a territory nobody can reach.
			if !known[id] {
				unknown.add(id)
				return nil
			}
			if err := tx.UpsertSyntaxonDistribution(id, a.Scheme, a.Code, occurrence); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			rep.Written++
			if occurrence == domain.OccurrenceUncertain {
				rep.Uncertain++
				return nil
			}
			rep.Verified++
			return nil
		})
}

func readCoverageRows(ctx context.Context, tx output.IngestTx, coveragePath string,
	known map[string]bool, unknown *unknownTracker, rep *SyntaxonDistributionReport) error {
	dir, file := splitCSVPath(coveragePath)
	skip := newRowSkipper(&rep.SkippedRows, file, "syntaxon distribution coverage")

	return readAll(ctx, dir, file, ',', []string{colSyntaxonID, colAreaScheme}, skip,
		func(idx map[string]int, row []string, line int) error {
			id := row[idx[colSyntaxonID]]
			scheme := row[idx[colAreaScheme]]
			if id == "" || scheme == "" {
				skip(line, fmt.Errorf("incomplete coverage row: syntaxon %q, scheme %q", id, scheme))
				return nil
			}
			if !domain.IsKnownAreaScheme(scheme) {
				skip(line, fmt.Errorf("area scheme %q is not one of %v", scheme, domain.KnownAreaSchemes()))
				return nil
			}
			if !known[id] {
				unknown.add(id)
				return nil
			}
			if err := tx.UpsertSyntaxonDistributionCoverage(id, scheme); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			rep.Covered++
			return nil
		})
}
