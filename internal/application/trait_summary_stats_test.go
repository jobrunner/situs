package application

import (
	"math"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func f64p(v float64) *float64 { return &v }

// tv builds one EIVE-shaped trait value; nw nil means "no niche width".
func tv(value float64, nw *float64) domain.TraitValue {
	return domain.TraitValue{Vocab: "eive", VocabVersion: "1.0", Dim: "M", Value: value, NicheWidth: nw}
}

func TestSummarizeDimension_MeanMinMaxAndCounts(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{
		tv(4, nil), tv(6, nil), tv(8, nil),
	}, 5)

	if got.Mean != 6 {
		t.Errorf("Mean = %v, want 6", got.Mean)
	}
	if got.Min != 4 || got.Max != 8 {
		t.Errorf("Min/Max = %v/%v, want 4/8", got.Min, got.Max)
	}
	if got.N != 3 {
		t.Errorf("N = %d, want 3", got.N)
	}
	// 5 known species, 3 of them carried a value here.
	if got.NMissing != 2 {
		t.Errorf("NMissing = %d, want 2", got.NMissing)
	}
}

// Sample standard deviation (n-1), the right one for a vegetation record
// understood as a sample.
func TestSummarizeDimension_SampleStandardDeviation(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{tv(2, nil), tv(4, nil), tv(6, nil)}, 3)

	if got.SD == nil {
		t.Fatal("SD is nil, want a value for n=3")
	}
	if math.Abs(*got.SD-2) > 1e-9 {
		t.Errorf("SD = %v, want 2", *got.SD)
	}
}

// One value has no spread that can be computed. Reporting 0 would be a
// claim about the data that the data does not support.
func TestSummarizeDimension_SDAbsentForSingleValue(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{tv(5, nil)}, 1)

	if got.SD != nil {
		t.Errorf("SD = %v, want nil for n=1", *got.SD)
	}
	if got.Mean != 5 {
		t.Errorf("Mean = %v, want 5", got.Mean)
	}
}

// Weight is 1/niche_width: a narrow niche is the more precise indicator.
// Values 4 (width 1) and 8 (width 2) weigh 1 and 0.5, so the mean is
// (4*1 + 8*0.5) / 1.5 = 5.333...
func TestSummarizeDimension_NicheWeightedMean(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{
		tv(4, f64p(1)), tv(8, f64p(2)),
	}, 2)

	if got.MeanNicheWeighted == nil {
		t.Fatal("MeanNicheWeighted is nil, want a value when niche widths are present")
	}
	if math.Abs(*got.MeanNicheWeighted-16.0/3.0) > 1e-9 {
		t.Errorf("MeanNicheWeighted = %v, want %v", *got.MeanNicheWeighted, 16.0/3.0)
	}
	// The unweighted mean stays what it was.
	if got.Mean != 6 {
		t.Errorf("Mean = %v, want 6", got.Mean)
	}
}

// Tichý and Midolo carry no niche widths — the field must be ABSENT, never
// filled with the unweighted mean, or a client could not tell them apart.
func TestSummarizeDimension_NoWeightedMeanWithoutNicheWidths(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{tv(4, nil), tv(8, nil)}, 2)

	if got.MeanNicheWeighted != nil {
		t.Errorf("MeanNicheWeighted = %v, want nil when no value carries a niche width", *got.MeanNicheWeighted)
	}
}

// A non-positive niche width would divide by zero. Such a value is excluded
// and counted, never repaired with an invented substitute.
func TestSummarizeDimension_NonPositiveNicheWidthExcludedAndCounted(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{
		tv(4, f64p(1)), tv(8, f64p(0)), tv(6, f64p(-1)),
	}, 3)

	if got.NExcludedWeighted != 2 {
		t.Errorf("NExcludedWeighted = %d, want 2", got.NExcludedWeighted)
	}
	if got.MeanNicheWeighted == nil {
		t.Fatal("MeanNicheWeighted is nil; the one usable value must still produce a weighted mean")
	}
	if *got.MeanNicheWeighted != 4 {
		t.Errorf("MeanNicheWeighted = %v, want 4 (only the width-1 value counts)", *got.MeanNicheWeighted)
	}
	// All three still count for the unweighted statistics.
	if got.N != 3 {
		t.Errorf("N = %d, want 3", got.N)
	}
}

// Every niche width unusable: no weighted mean at all, but all of them
// counted as excluded.
func TestSummarizeDimension_AllNicheWidthsUnusable(t *testing.T) {
	got := summarizeDimension([]domain.TraitValue{tv(4, f64p(0)), tv(8, f64p(0))}, 2)

	if got.MeanNicheWeighted != nil {
		t.Errorf("MeanNicheWeighted = %v, want nil", *got.MeanNicheWeighted)
	}
	if got.NExcludedWeighted != 2 {
		t.Errorf("NExcludedWeighted = %d, want 2", got.NExcludedWeighted)
	}
}
