package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// pipeDelim is the field separator of the three canonical trait CSVs
// (taxon|vocab|vocab_version|dim|value|niche_width|n_systems) — set by the
// pipelines' own convert.py (measured, not assumed: the transferred files
// use "|" despite the .csv extension). A comma-delimited reader would
// silently misparse every row.
const pipeDelim = '|'

// VocabReport is one vocabulary's slice of TraitReport.
type VocabReport struct {
	Rows       int
	Resolved   int
	Unresolved int
}

// TraitReport summarizes one trait ingest run across all three canonical
// CSVs, resolved through ONE hostus.Resolve() call — the distinct taxa
// across all vocabularies are collected first, not resolved per file.
type TraitReport struct {
	Rows       int
	Resolved   int
	Unresolved int
	PerVocab   map[string]VocabReport
	// Skipped names the vocabularies whose CSV file was missing from
	// csvPaths — trait data is extra information, like distribution, so a
	// missing file does not abort the run.
	Skipped []string
}

type traitRow struct {
	taxon        string
	vocab        string
	vocabVersion string
	dim          string
	value        float64
	nicheWidth   *float64
	nSystems     *int
}

// parseFloat is the required-value counterpart to parseOptionalFloat: the
// "value" column is never absent in a well-formed trait row.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// readTraitRows parses one canonical pipe-delimited trait CSV, skipping
// (and counting) any row with the wrong field count or an unparseable
// value — the same tolerance as every other ingest file in this package.
func readTraitRows(ctx context.Context, csvPath string, skip rowSkipper) ([]traitRow, error) {
	dir, file := filepath.Split(csvPath)
	var rows []traitRow
	err := readAll(ctx, dir, file, pipeDelim,
		[]string{"taxon", "vocab", "vocab_version", "dim", "value", "niche_width", "n_systems"}, skip,
		func(idx map[string]int, row []string, line int) error {
			value, perr := parseFloat(row[idx["value"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			nicheWidth, perr := parseOptionalFloat(row[idx["niche_width"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			nSystems, perr := parseOptionalInt(row[idx["n_systems"]])
			if perr != nil {
				skip(line, perr)
				return nil
			}
			rows = append(rows, traitRow{
				taxon:        row[idx["taxon"]],
				vocab:        row[idx["vocab"]],
				vocabVersion: row[idx["vocab_version"]],
				dim:          row[idx["dim"]],
				value:        value,
				nicheWidth:   nicheWidth,
				nSystems:     nSystems,
			})
			return nil
		})
	return rows, err
}

// distinctTaxa returns the deduplicated taxon names across rows, sorted so
// batch composition is reproducible run to run — same rationale as
// species_ingest.go's distinctNames.
func distinctTaxa(rows []traitRow) []string {
	names := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		names[r.taxon] = struct{}{}
	}
	distinct := make([]string, 0, len(names))
	for n := range names {
		distinct = append(distinct, n)
	}
	sort.Strings(distinct)
	return distinct
}

// IngestTraits loads the three canonical trait CSVs named in csvPaths
// (keyed by vocabulary, e.g. {"eive": ".../eive_traits.csv", ...}) into
// repo, crosswalking every distinct taxon across ALL THREE files to a
// hostus concept ID in one Resolve call. A missing file is skipped and
// recorded in Skipped, never aborting the run. A resolver failure aborts
// the whole ingest — otherwise every row would be misrecorded as
// "unresolvable" instead of "hostus was down", the same distinction
// IngestSpeciesRoles already makes. An unresolved taxon's row is dropped
// (not stored with a nil concept id): trait_value's primary key requires
// one, unlike species_role's.
func IngestTraits(ctx context.Context, repo output.Repository, resolver output.NameResolver,
	csvPaths map[string]string) (TraitReport, error) {
	vocabKeys := make([]string, 0, len(csvPaths))
	for vocab := range csvPaths {
		vocabKeys = append(vocabKeys, vocab)
	}
	sort.Strings(vocabKeys) // reproducible order, not map iteration order

	rep := TraitReport{PerVocab: map[string]VocabReport{}, Skipped: []string{}}
	byVocab := map[string][]traitRow{}
	var all []traitRow
	skipped := 0
	for _, vocab := range vocabKeys {
		path := csvPaths[vocab]
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			rep.Skipped = append(rep.Skipped, vocab)
			continue
		}
		_, file := filepath.Split(path)
		skip := newRowSkipper(&skipped, file, "trait value")
		rows, err := readTraitRows(ctx, path, skip)
		if err != nil {
			return TraitReport{}, fmt.Errorf("reading %s trait CSV: %w", vocab, err)
		}
		byVocab[vocab] = rows
		all = append(all, rows...)
	}

	resolved, err := resolver.Resolve(ctx, distinctTaxa(all))
	if err != nil {
		return TraitReport{}, fmt.Errorf("resolving trait taxa via hostus: %w", err)
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return TraitReport{}, fmt.Errorf("beginning trait ingest transaction: %w", err)
	}

	for _, vocab := range vocabKeys {
		rows, ok := byVocab[vocab]
		if !ok {
			continue
		}
		vr, verr := upsertTraitRows(tx, vocab, rows, resolved)
		if verr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return TraitReport{}, fmt.Errorf("%w (rollback also failed: %w)", verr, rbErr)
			}
			return TraitReport{}, verr
		}
		rep.PerVocab[vocab] = vr
		rep.Rows += vr.Rows
		rep.Resolved += vr.Resolved
		rep.Unresolved += vr.Unresolved
	}

	if err := tx.Commit(); err != nil {
		return TraitReport{}, fmt.Errorf("committing trait ingest transaction: %w", err)
	}
	return rep, nil
}

// upsertTraitRows writes vocab's rows via tx, tallying vr accordingly, and
// records the vocabulary once (with the version its own rows carry) so
// trait_vocabulary reflects a file that was processed even if every single
// taxon in it failed to resolve.
func upsertTraitRows(tx output.IngestTx, vocab string, rows []traitRow, resolved map[string]string) (VocabReport, error) {
	vr := VocabReport{Rows: len(rows)}
	var version string
	for _, r := range rows {
		version = r.vocabVersion
		conceptID, ok := resolved[r.taxon]
		if !ok {
			vr.Unresolved++
			continue
		}
		tv := domain.TraitValue{
			Vocab:        r.vocab,
			VocabVersion: r.vocabVersion,
			Dim:          domain.TraitDim(r.dim),
			Value:        r.value,
			NicheWidth:   r.nicheWidth,
			NSystems:     r.nSystems,
		}
		if err := tx.UpsertTraitValue(conceptID, tv); err != nil {
			return VocabReport{}, err
		}
		vr.Resolved++
	}
	if len(rows) > 0 {
		if err := tx.UpsertTraitVocabulary(vocab, version); err != nil {
			return VocabReport{}, err
		}
	}
	return vr, nil
}
