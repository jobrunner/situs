package application

import (
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// seedFormationLabels gives the navigation world its German formation labels:
// exactly what data/localizations-de-syntaxa.csv ships, provenance situs.
func seedFormationLabels(repo *fakeRepo) {
	repo.localizations = append(repo.localizations,
		domain.Localization{EntityType: "syntaxon", EntityKey: "C", Lang: "de", Field: "name",
			Value: "Vegetation der nemoralen Waldzone", Source: "situs", Provenance: "situs"},
		domain.Localization{EntityType: "syntaxon", EntityKey: "R", Lang: "de", Field: "name",
			Value: "Epigäische Moos- und Flechtenvegetation", Source: "situs", Provenance: "situs"},
		domain.Localization{EntityType: "syntaxon", EntityKey: "R", Lang: "de", Field: "vernacular",
			Value: "Bodenmoos- und Bodenflechtenvegetation", Source: "situs", Provenance: "situs"},
	)
}

func TestSyntaxaByRank_OverlaysTheGermanFormationLabel(t *testing.T) {
	repo := seedNavRepo(t)
	seedFormationLabels(repo)
	q := NewQueryService(repo)

	got, err := q.SyntaxaByRank(t.Context(), domain.SyntaxonRankFormation, "", "de", input.SyntaxonAreaFilter{})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	byID := map[string]input.SyntaxonRef{}
	for _, r := range got {
		byID[r.ID] = r
	}
	c := byID["C"]
	if c.NameDE == nil {
		t.Fatal("formation C carries no name_de")
	}
	if c.NameDE.Value != "Vegetation der nemoralen Waldzone" {
		t.Errorf("C name_de = %q", c.NameDE.Value)
	}
	// The overlay is additive: the scientific English name stays the identity.
	if c.Name != "Vegetation of the nemoral forest zone" {
		t.Errorf("C name = %q, the overlay replaced the identity", c.Name)
	}
	if c.NameDE.Provenance != "situs" {
		t.Errorf("C name_de provenance = %q, want situs", c.NameDE.Provenance)
	}
	if v := byID["R"].NameDE.Vernacular; v != "Bodenmoos- und Bodenflechtenvegetation" {
		t.Errorf("R vernacular = %q", v)
	}
}

// Without ?lang=de nothing is overlaid: the German label is an opt-in, exactly
// as on the habitat-type side.
func TestSyntaxaByRank_WithoutLangCarriesNoGermanLabel(t *testing.T) {
	repo := seedNavRepo(t)
	seedFormationLabels(repo)
	q := NewQueryService(repo)

	got, err := q.SyntaxaByRank(t.Context(), domain.SyntaxonRankFormation, "", "", input.SyntaxonAreaFilter{})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	for _, r := range got {
		if r.NameDE != nil {
			t.Errorf("%s carries name_de without lang=de", r.ID)
		}
	}
}

// A syntaxon with no localization row of its own stays untranslated rather than
// borrowing its formation's label. This guards the CODE, not the data: the
// fixture gives labels to the formations only, so an implementation that walked
// up the ancestor path for a fallback would light this up. Which ranks actually
// carry a translation is a property of data/localizations-de-syntaxa.csv and is
// checked there (cmd/situs/curated_test.go), not here.
func TestSyntaxaByRank_UntranslatedRankStaysUntranslated(t *testing.T) {
	repo := seedNavRepo(t)
	seedFormationLabels(repo)
	q := NewQueryService(repo)

	got, err := q.SyntaxaByRank(t.Context(), domain.SyntaxonRankAlliance, "", "de", input.SyntaxonAreaFilter{})
	if err != nil {
		t.Fatalf("SyntaxaByRank: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no alliances in the fixture")
	}
	for _, r := range got {
		if r.NameDE != nil {
			t.Errorf("alliance %s carries name_de %q, but only formations are translated", r.ID, r.NameDE.Value)
		}
	}
}

// The detail route is where the breadcrumb is printed, so the ancestors carry
// the label too: without it a German breadcrumb would end in an English root.
func TestSyntaxon_OverlaysSelfAndAncestors(t *testing.T) {
	repo := seedNavRepo(t)
	seedFormationLabels(repo)
	q := NewQueryService(repo)

	got, err := q.Syntaxon(t.Context(), "CA01A", "de")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.NameDE != nil {
		t.Errorf("alliance CA01A carries name_de %q", got.NameDE.Value)
	}
	if len(got.Ancestors) == 0 {
		t.Fatal("no ancestors")
	}
	root := got.Ancestors[0]
	if root.ID != "C" {
		t.Fatalf("outermost ancestor = %s, want C", root.ID)
	}
	if root.NameDE == nil || root.NameDE.Value != "Vegetation der nemoralen Waldzone" {
		t.Errorf("breadcrumb root carries no German label: %+v", root.NameDE)
	}
}

// The formation's own detail view, and its children on the way down.
func TestSyntaxon_OverlaysTheFormationItself(t *testing.T) {
	repo := seedNavRepo(t)
	seedFormationLabels(repo)
	q := NewQueryService(repo)

	got, err := q.Syntaxon(t.Context(), "C", "de")
	if err != nil {
		t.Fatalf("Syntaxon: %v", err)
	}
	if got.NameDE == nil || got.NameDE.Value != "Vegetation der nemoralen Waldzone" {
		t.Fatalf("formation C carries no name_de: %+v", got.NameDE)
	}
	for _, c := range got.Children {
		if c.NameDE != nil {
			t.Errorf("child %s carries name_de, but only formations are translated", c.ID)
		}
	}
}

// A broken label lookup must surface, not be swallowed into an untranslated
// answer: a silently English list looks exactly like an index nobody has
// translated yet, and that is the one failure nobody would ever notice.
func TestSyntaxonLabels_RepositoryFailureSurfaces(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(q *QueryService) error
	}{
		{"SyntaxaByRank", func(q *QueryService) error {
			_, err := q.SyntaxaByRank(t.Context(), domain.SyntaxonRankFormation, "", "de",
				input.SyntaxonAreaFilter{})
			return err
		}},
		{"Syntaxon", func(q *QueryService) error {
			_, err := q.Syntaxon(t.Context(), "CA01A", "de")
			return err
		}},
	} {
		repo := seedNavRepo(t)
		repo.localizationErr = errBoom
		err := tc.call(NewQueryService(repo))
		if err == nil {
			t.Errorf("%s verschluckt den Fehler der Label-Abfrage", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), "German syntaxon labels") {
			t.Errorf("%s: Fehler = %q, nennt den Schritt nicht", tc.name, err)
		}
	}
}
