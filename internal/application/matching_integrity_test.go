package application

import (
	"context"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// Die Konzept-IDs sind die echten aus dem Index; die Stetigkeiten sind der
// Groessenordnung nach uebernommen, damit der Fixture dasselbe Verhalten zeigt
// wie der gepinnte Index.
const (
	conceptFagus    = "wcvp:concept:83891"
	conceptAnemone  = "wcvp:concept:2638482"
	conceptGalium   = "wcvp:concept:87028"
	conceptStoerart = "wcvp:concept:urtica"
)

// newMatchServiceFromFixture baut fuenf Waldtypen nach: T17 fuehrt alle drei
// Arten mit hoher Stetigkeit, T1F und T1E je zwei, T18 und T32 je eine.
func newMatchServiceFromFixture(t *testing.T) *QueryService {
	t.Helper()
	repo := newFakeRepo()
	repo.typologies = []domain.Typology{{ID: "eunis@2021", Scheme: "eunis", Version: "2021"}}
	level := 3
	typ := func(code string) domain.HabitatTypeKey {
		k := domain.HabitatTypeKey{Typology: "eunis@2021", Code: code}
		repo.types = append(repo.types, domain.HabitatType{Key: k, Level: &level, NameEN: code})
		return k
	}
	art := func(k domain.HabitatTypeKey, cid string, constancy float64) {
		id, c := cid, constancy
		repo.speciesRoles = append(repo.speciesRoles, domain.SpeciesRole{
			Key: k, ConceptID: &id, VerbatimName: cid, Role: "constant",
			Constancy: &c, Provenance: "observed",
		})
	}
	t17, t1f, t1e, t18, t32 := typ("T17"), typ("T1F"), typ("T1E"), typ("T18"), typ("T32")
	// Die Stoerart ist dem Index bekannt, nur nicht in den Waldtypen: eine
	// Ruderale wie Urtica dioica steht in der Nitrophytenflur. Genau so sieht
	// der Fall im Feld aus, und nur so ist "of" die Zahl der bekannten Arten.
	v38 := typ("V38")
	art(v38, conceptStoerart, 70)
	art(t17, conceptFagus, 99)
	art(t17, conceptAnemone, 60)
	art(t17, conceptGalium, 55)
	art(t1f, conceptFagus, 40)
	art(t1f, conceptGalium, 30)
	art(t1e, conceptAnemone, 50)
	art(t1e, conceptGalium, 20)
	art(t18, conceptFagus, 80)
	art(t32, conceptFagus, 30)
	return NewQueryService(repo)
}

// Die namentliche Zusage der Spec: drei Arten des Waldmeister-Buchenwalds
// muessen T17 auf Rang 1 bringen, mit Abstand zum zweiten. Gegen den echten
// Index gemessen betraegt der Abstand 1,23.
func TestMatching_FagusAnemoneGaliumErgibtT17(t *testing.T) {
	svc := newMatchServiceFromFixture(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{conceptFagus, conceptAnemone, conceptGalium},
		Typology:   "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) == 0 || got.Matches[0].Code != "T17" {
		t.Fatalf("Rang 1 = %v, erwartet T17", got.Matches)
	}
	if len(got.Matches) > 1 && got.Matches[0].Score-got.Matches[1].Score < 0.5 {
		t.Errorf("Abstand zum zweiten nur %.3f; die Spec verlangt einen deutlichen",
			got.Matches[0].Score-got.Matches[1].Score)
	}
}

// Eine Stoerart darf das richtige Habitat nicht aus der Liste werfen — genau
// das leistet der harte Filter nicht, und genau deshalb gibt es MatchMiss.
// Gegen den echten Index bleibt T17 auch mit Urtica dioica auf Rang 1.
func TestMatching_StoerartWirftDasRichtigeHabitatNichtHeraus(t *testing.T) {
	svc := newMatchServiceFromFixture(t)

	got, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{conceptFagus, conceptAnemone, conceptGalium, conceptStoerart},
		Typology:   "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(got.Matches) == 0 || got.Matches[0].Code != "T17" {
		t.Fatalf("mit einer Stoerart ist Rang 1 = %v, erwartet weiterhin T17", got.Matches)
	}
	if got.Matches[0].Matched != 3 || got.Matches[0].Of != 4 {
		t.Errorf("matched/of = %d/%d, erwartet 3/4", got.Matches[0].Matched, got.Matches[0].Of)
	}
}

// Der Gegenbeweis zum verworfenen Entwurf: haette eine unpassende Art
// ausschliessende Wirkung, waere T17 nach der Stoerart verschwunden. Der
// MatchMiss-Term sorgt dafuer, dass sie nur abwertet — der Abstand schrumpft,
// die Ordnung bleibt.
func TestMatching_StoerartWertetAbStattAuszuschliessen(t *testing.T) {
	svc := newMatchServiceFromFixture(t)
	ohne, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{conceptFagus, conceptAnemone, conceptGalium},
		Typology:   "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	mit, err := svc.MatchHabitatTypes(context.Background(), input.MatchRequest{
		ConceptIDs: []string{conceptFagus, conceptAnemone, conceptGalium, conceptStoerart},
		Typology:   "eunis@2021", Level: 3, Limit: 5,
	})
	if err != nil {
		t.Fatalf("MatchHabitatTypes: %v", err)
	}
	if len(ohne.Matches) != len(mit.Matches) {
		t.Errorf("die Stoerart aendert die Zahl der Kandidaten: %d → %d",
			len(ohne.Matches), len(mit.Matches))
	}
	if mit.Matches[0].Score >= ohne.Matches[0].Score {
		t.Error("die Stoerart senkt den Score nicht; sie muss abwerten")
	}
}
