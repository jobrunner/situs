package domain_test

import (
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

func TestSyntaxonKonstantenWerte(t *testing.T) {
	for got, want := range map[string]string{
		domain.SyntaxonRankFormation:   "formation",
		domain.SyntaxonSourceEVC:       "evc",
		domain.SyntaxonSourceEUNIS:     "eunis",
		domain.ParentProvenanceDerived: "derived",
		domain.LifeFormBryophyteLichen: "bryophyte_lichen",
	} {
		if got != want {
			t.Errorf("Konstante = %q, erwartet %q", got, want)
		}
	}
}
