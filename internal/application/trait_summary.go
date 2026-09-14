package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// SpeciesTraitSummary aggregates a species list's indicator values, strictly
// per vocabulary and dimension.
//
// Never across vocabularies: EIVE's 0–10 and Tichý's 1–12 measure different
// things on different scales, so a common mean would be an invented number.
func (q *QueryService) SpeciesTraitSummary(ctx context.Context, conceptIDs []string) (input.TraitSummary, error) {
	if len(conceptIDs) == 0 {
		return input.TraitSummary{}, fmt.Errorf("concept_ids must hold at least one id: %w", input.ErrInvalidQuery)
	}

	// A foreign backbone is the caller's fault and needs no index read; the
	// rest is asked about in one query.
	wanted := make([]string, 0, len(conceptIDs))
	unknown := []input.UnknownConcept{}
	for _, id := range conceptIDs {
		if !strings.HasPrefix(id, indexBackbone+":") {
			unknown = append(unknown, input.UnknownConcept{ConceptID: id, Reason: input.ReasonUnknownBackbone})
			continue
		}
		wanted = append(wanted, id)
	}

	traits := map[string][]domain.TraitValue{}
	if len(wanted) > 0 {
		var err error
		traits, err = q.repo.TraitsForConcepts(ctx, wanted)
		if err != nil {
			return input.TraitSummary{}, fmt.Errorf("reading traits of %d concepts: %w", len(wanted), err)
		}
	}
	for _, id := range wanted {
		if _, ok := traits[id]; !ok {
			unknown = append(unknown, input.UnknownConcept{ConceptID: id, Reason: input.ReasonUnknownConcept})
		}
	}

	return input.TraitSummary{
		Requested:    len(conceptIDs),
		Known:        len(traits),
		Unknown:      unknown,
		Vocabularies: summarizeVocabularies(traits),
	}, nil
}

// summarizeVocabularies buckets every value by (vocab, dim) and summarizes
// each bucket. A dimension nobody carried a value for never gets a bucket,
// so it is absent from the answer rather than a row of zeroes.
func summarizeVocabularies(traits map[string][]domain.TraitValue) map[string]input.VocabSummary {
	type bucket struct {
		version string
		byDim   map[string][]domain.TraitValue
	}
	buckets := map[string]*bucket{}
	for _, values := range traits {
		for _, v := range values {
			b, ok := buckets[v.Vocab]
			if !ok {
				b = &bucket{version: v.VocabVersion, byDim: map[string][]domain.TraitValue{}}
				buckets[v.Vocab] = b
			}
			b.byDim[string(v.Dim)] = append(b.byDim[string(v.Dim)], v)
		}
	}

	known := len(traits)
	out := make(map[string]input.VocabSummary, len(buckets))
	for vocab, b := range buckets {
		dims := make(map[string]input.DimensionSummary, len(b.byDim))
		for dim, values := range b.byDim {
			dims[dim] = summarizeDimension(values, known)
		}
		out[vocab] = input.VocabSummary{VocabVersion: b.version, Dimensions: dims}
	}
	return out
}
