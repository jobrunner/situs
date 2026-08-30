package application

import (
	"context"
	"fmt"
	"slices"

	"github.com/jobrunner/situs/internal/ports/input"
)

// Traits returns a concept's indicator values, grouped per vocabulary,
// never merged. An empty vocab answers every ingested vocabulary; a
// non-empty one that the index has no data for is INVALID_QUERY — the same
// distinction areaLookup already makes for ?area=.
func (q *QueryService) Traits(ctx context.Context, conceptID, vocab string) ([]input.TraitSetView, error) {
	var vocabs []string
	if vocab != "" {
		known, err := q.repo.KnownVocabs(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing known trait vocabularies: %w", err)
		}
		if !slices.Contains(known, vocab) {
			return nil, fmt.Errorf("trait vocabulary %q: %w", vocab, input.ErrUnknownVocab)
		}
		vocabs = []string{vocab}
	}

	sets, err := q.repo.Traits(ctx, conceptID, vocabs)
	if err != nil {
		return nil, fmt.Errorf("fetching traits of %q: %w", conceptID, err)
	}
	out := make([]input.TraitSetView, 0, len(sets))
	for _, s := range sets {
		values := make([]input.TraitValueView, 0, len(s.Values))
		for _, v := range s.Values {
			values = append(values, input.TraitValueView{
				Dim: string(v.Dim), Value: v.Value, NicheWidth: v.NicheWidth, NSystems: v.NSystems,
			})
		}
		out = append(out, input.TraitSetView{Vocab: s.Vocab, VocabVersion: s.VocabVersion, Values: values})
	}
	return out, nil
}
