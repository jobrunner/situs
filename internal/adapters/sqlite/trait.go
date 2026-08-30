package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
)

// UpsertTraitValue writes one trait_value row for conceptID. Idempotent
// like every other Upsert here: a repinned vocabulary is simply
// re-ingested.
func (t *ingestTx) UpsertTraitValue(conceptID string, tv domain.TraitValue) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO trait_value (concept_id, vocab, vocab_version, dim, value, niche_width, n_systems)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(concept_id, vocab, vocab_version, dim) DO UPDATE SET
		   value=excluded.value, niche_width=excluded.niche_width, n_systems=excluded.n_systems`,
		conceptID, tv.Vocab, tv.VocabVersion, string(tv.Dim), tv.Value, tv.NicheWidth, tv.NSystems)
	if err != nil {
		return fmt.Errorf("sqlite: upserting trait value %s/%s/%s for %s: %w",
			tv.Vocab, tv.VocabVersion, tv.Dim, conceptID, err)
	}
	return nil
}

// UpsertTraitVocabulary records that vocab/version was (re-)ingested. Pure
// ingest metadata, no factual content for the reader.
func (t *ingestTx) UpsertTraitVocabulary(vocab, version string) error {
	_, err := t.tx.ExecContext(t.ctx,
		`INSERT INTO trait_vocabulary (vocab, version, ingested_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(vocab, version) DO UPDATE SET ingested_at=excluded.ingested_at`,
		vocab, version)
	if err != nil {
		return fmt.Errorf("sqlite: upserting trait vocabulary %s %s: %w", vocab, version, err)
	}
	return nil
}

// Traits returns every domain.TraitSet situs holds for conceptID, grouped
// per vocabulary, never mixed. An empty vocabs slice means every ingested
// vocabulary.
func (d *DB) Traits(ctx context.Context, conceptID string, vocabs []string) ([]domain.TraitSet, error) {
	query := `SELECT vocab, vocab_version, dim, value, niche_width, n_systems
	          FROM trait_value WHERE concept_id = ?`
	args := []any{conceptID}
	if len(vocabs) > 0 {
		// Only placeholders are generated here, never values — the vocab
		// names stay arguments, so this is not SQL construction from input
		// (gosec G201/G202), same idiom as appendAreasForChunk.
		placeholders := strings.Repeat(",?", len(vocabs))[1:]
		query += ` AND vocab IN (` + placeholders + `)`
		for _, v := range vocabs {
			args = append(args, v)
		}
	}
	query += ` ORDER BY vocab, vocab_version, dim`

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading traits of %s: %w", conceptID, err)
	}
	defer func() { _ = rows.Close() }()

	sets := map[string]*domain.TraitSet{}
	var order []string
	for rows.Next() {
		var vocab, vocabVersion, dim string
		var tv domain.TraitValue
		if err := rows.Scan(&vocab, &vocabVersion, &dim, &tv.Value, &tv.NicheWidth, &tv.NSystems); err != nil {
			return nil, fmt.Errorf("sqlite: scanning trait value of %s: %w", conceptID, err)
		}
		tv.Vocab, tv.VocabVersion, tv.Dim = vocab, vocabVersion, domain.TraitDim(dim)
		key := vocab + "@" + vocabVersion
		set, ok := sets[key]
		if !ok {
			set = &domain.TraitSet{Vocab: vocab, VocabVersion: vocabVersion}
			sets[key] = set
			order = append(order, key)
		}
		set.Values = append(set.Values, tv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating traits of %s: %w", conceptID, err)
	}

	out := make([]domain.TraitSet, 0, len(order))
	for _, key := range order {
		out = append(out, *sets[key])
	}
	return out, nil
}

// KnownVocabs lists the distinct trait vocabularies the index has data for.
// A ?vocab= filter is validated against this, same role as KnownAreaCodes
// plays for ?area=.
func (d *DB) KnownVocabs(ctx context.Context) ([]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT DISTINCT vocab FROM trait_vocabulary ORDER BY vocab`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading known vocabs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []string{}
	for rows.Next() {
		var vocab string
		if err := rows.Scan(&vocab); err != nil {
			return nil, fmt.Errorf("sqlite: scanning vocab: %w", err)
		}
		out = append(out, vocab)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating known vocabs: %w", err)
	}
	return out, nil
}
