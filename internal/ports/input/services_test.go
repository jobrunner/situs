package input_test

import (
	"testing"

	"github.com/jobrunner/situs/internal/ports/input"
)

// Active decides whether the area machinery runs at all, so an empty code must
// read as "no filter asked for" and never as "filter on the empty area".
func TestAreaFilterActive(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter input.AreaFilter
		want   bool
	}{
		{"no code at all", input.AreaFilter{}, false},
		{"a code", input.AreaFilter{Code: "GER"}, true},
		{"only_in_area without a code is still inactive", input.AreaFilter{OnlyInArea: true}, false},
		{"a code with only_in_area", input.AreaFilter{Code: "GER", OnlyInArea: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.filter.Active(); got != tc.want {
				t.Errorf("AreaFilter%+v.Active() = %t, want %t", tc.filter, got, tc.want)
			}
		})
	}
}
