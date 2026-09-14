package application

import (
	"math"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// summarizeDimension computes one dimension's statistics over the values of
// one vocabulary. Pure: no I/O, no repository, no context.
//
// known is how many of the requested concepts the index had ANY trait data
// for, so NMissing can say how many of them said nothing about THIS
// dimension. Callers pass it in rather than having it derived here, because
// only the caller knows the size of the species list.
//
// The caller guarantees values is non-empty and holds one vocabulary's
// values for one dimension.
func summarizeDimension(values []domain.TraitValue, known int) input.DimensionSummary {
	sum, min, max := 0.0, values[0].Value, values[0].Value
	for _, v := range values {
		sum += v.Value
		min = math.Min(min, v.Value)
		max = math.Max(max, v.Value)
	}
	n := len(values)
	mean := sum / float64(n)

	out := input.DimensionSummary{
		Mean: mean, Min: min, Max: max, N: n, NMissing: known - n,
	}
	if sd, ok := sampleSD(values, mean); ok {
		out.SD = &sd
	}
	weighted, excluded, ok := nicheWeightedMean(values)
	if ok {
		out.MeanNicheWeighted = &weighted
	}
	out.NExcludedWeighted = excluded
	return out
}

// sampleSD is the sample standard deviation (n-1). A single value has no
// computable spread — reporting 0 would state that the data agrees with
// itself, which is not what one measurement shows.
func sampleSD(values []domain.TraitValue, mean float64) (float64, bool) {
	if len(values) < 2 {
		return 0, false
	}
	sumSq := 0.0
	for _, v := range values {
		d := v.Value - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(values)-1)), true
}

// nicheWeightedMean weights each value by 1/niche_width: a narrow niche is a
// more precise site indicator than a generalist's broad one.
//
// Values without a niche width (Tichý, Midolo never carry one) contribute
// nothing and are NOT counted as excluded — the vocabulary simply does not
// offer the measure. A width that is present but not positive IS counted:
// that is a data defect worth seeing, and dividing by it would be a panic
// or an infinity. Measured on the pinned index this never occurs.
//
// ok is false when no value could be weighted at all; the caller then omits
// the field rather than substituting the unweighted mean.
func nicheWeightedMean(values []domain.TraitValue) (mean float64, excluded int, ok bool) {
	sumWeighted, sumWeights := 0.0, 0.0
	for _, v := range values {
		if v.NicheWidth == nil {
			continue
		}
		if *v.NicheWidth <= 0 {
			excluded++
			continue
		}
		w := 1 / *v.NicheWidth
		sumWeighted += v.Value * w
		sumWeights += w
	}
	if sumWeights == 0 {
		return 0, excluded, false
	}
	return sumWeighted / sumWeights, excluded, true
}
