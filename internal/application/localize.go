package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// deLang/nameField pin the (lang, field) pair DeriveGermanLabels looks up
// and writes — the only pair this foundation derives.
const (
	deLang    = "de"
	nameField = "name"
)

// The provenance vocabulary of localization: an overlay is either the official
// wording of a source, a curated one, or computed from a crosswalk.
const (
	provenanceOfficial = "official"
	provenanceCurated  = "curated"
	provenanceDerived  = "derived"
)

// IngestLocalizations loads csvPath (localizations.csv:
// entity_type,entity_key,lang,field,value,source,provenance) into repo. No
// source in this foundation produces this file yet — the amtliche German
// Annex I names come from EUR-Lex, pinned later (spec open point 6) — so a
// missing file is "no localizations", not an error: it is logged at info
// level and the count is 0.
func IngestLocalizations(ctx context.Context, repo output.Repository, csvPath string) (int, error) {
	if _, err := os.Stat(csvPath); errors.Is(err, os.ErrNotExist) {
		slog.Info("no localizations file, skipping", "path", csvPath)
		return 0, nil
	}

	dir, file := filepath.Split(csvPath)

	tx, err := repo.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("beginning localization ingest transaction: %w", err)
	}

	count, skipped := 0, 0
	skip := newRowSkipper(&skipped, file, "localization")
	err = readAll(ctx, dir, file,
		[]string{"entity_type", "entity_key", "lang", "field", "value", "source", "provenance"}, skip,
		func(idx map[string]int, row []string, line int) error {
			provenance := row[idx["provenance"]]
			if provenance != provenanceOfficial && provenance != provenanceCurated && provenance != provenanceDerived {
				skip(line, fmt.Errorf("provenance %q is none of official/curated/derived", provenance))
				return nil
			}
			l := domain.Localization{
				EntityType: row[idx["entity_type"]],
				EntityKey:  row[idx["entity_key"]],
				Lang:       row[idx["lang"]],
				Field:      row[idx["field"]],
				Value:      row[idx["value"]],
				Source:     row[idx["source"]],
				Provenance: provenance,
			}
			if err := tx.UpsertLocalization(l); err != nil {
				return fmt.Errorf("%s:%d: %w", file, line, err)
			}
			count++
			return nil
		})
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return 0, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
		}
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing localization ingest transaction: %w", err)
	}
	return count, nil
}

// DeriveGermanLabels lends the official German Annex I name to every EUNIS
// (or other) type crosswalked to it with qualifier '=' — the only qualifier
// precise enough to lend a name (see domain.Qualifier.IsSame). It never
// overwrites an existing official or curated de/name label for the source
// type, and every derived row is marked Provenance: "derived" so it can never
// be mistaken for the official wording it was copied from.
func DeriveGermanLabels(ctx context.Context, repo output.Repository) (int, error) {
	crosswalks, err := repo.CrosswalksTo(ctx, "annex1")
	if err != nil {
		return 0, fmt.Errorf("fetching crosswalks to annex1: %w", err)
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("beginning derivation transaction: %w", err)
	}

	// derivedInThisRun guards against two '=' crosswalks from the same source
	// type to different Annex I targets: repo.Localization cannot see this
	// run's own uncommitted upserts, so without this a second such crosswalk
	// would upsert a second derived-annex1 row onto the same (entity,
	// field, source) slot, silently replacing the first while count still
	// counted both. First crosswalk (in CrosswalksTo's order) wins.
	count := 0
	derivedInThisRun := make(map[string]bool)
	for _, c := range crosswalks {
		if derivedInThisRun[c.From.String()] {
			continue
		}
		derived, err := deriveOne(ctx, repo, tx, c)
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return 0, fmt.Errorf("%w (rollback also failed: %w)", err, rbErr)
			}
			return 0, err
		}
		if derived {
			count++
			derivedInThisRun[c.From.String()] = true
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing derivation transaction: %w", err)
	}
	return count, nil
}

// deriveOne applies the derivation rule to one crosswalk: only '=' qualifies,
// an existing official/curated de/name for the source is never overwritten,
// and there must be an official/curated de/name on the Annex I target to
// copy. It reports whether a row was upserted.
func deriveOne(ctx context.Context, repo output.Repository, tx output.IngestTx, c domain.Crosswalk) (bool, error) {
	if !c.Qualifier.IsSame() {
		return false, nil
	}

	existing, err := repo.Localization(ctx, "habitat_type", c.From.String(), deLang, nameField)
	if err != nil {
		return false, fmt.Errorf("checking existing localization for %s: %w", c.From, err)
	}
	if hasOfficialOrCurated(existing) {
		return false, nil
	}

	target, err := repo.Localization(ctx, "habitat_type", c.To.String(), deLang, nameField)
	if err != nil {
		return false, fmt.Errorf("fetching Annex I name for %s: %w", c.To, err)
	}
	name, ok := officialOrCuratedName(target)
	if !ok {
		return false, nil
	}

	l := domain.Localization{
		EntityType:  "habitat_type",
		EntityKey:   c.From.String(),
		Lang:        deLang,
		Field:       nameField,
		Value:       name,
		Source:      "derived-annex1",
		Provenance:  provenanceDerived,
		DerivedFrom: fmt.Sprintf("%s qualifier==", c.To),
	}
	if err := tx.UpsertLocalization(l); err != nil {
		return false, fmt.Errorf("upserting derived label for %s: %w", c.From, err)
	}
	return true, nil
}

func hasOfficialOrCurated(ls []domain.Localization) bool {
	_, ok := officialOrCuratedName(ls)
	return ok
}

// officialOrCuratedName returns the value of an official or curated entry in
// ls, preferring official — repeated ingests of the same data must produce
// byte-identical derived labels, not a value that depends on ls's row order.
// Repository.Localization does order its rows, but this must hold for any
// implementation of the port, not just the sqlite one. A derived entry
// must never be treated as a source to derive from, nor as a reason to skip
// deriving.
func officialOrCuratedName(ls []domain.Localization) (string, bool) {
	var curated string
	var sawCurated bool
	for _, l := range ls {
		if l.Provenance == provenanceOfficial {
			return l.Value, true
		}
		if l.Provenance == provenanceCurated && !sawCurated {
			curated, sawCurated = l.Value, true
		}
	}
	return curated, sawCurated
}
