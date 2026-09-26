package domain_test

import (
	"math"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// constancy IS P(species | habitat): 99 means the species occurs in 99 % of
// this type's relevés. A species the type does not list costs MISS, which is
// the whole point — a hard AND filter loses the true habitat in 96 % of cases
// as soon as one unlisted species is named.
func TestScoreCandidate_FehlendeArtKostetMissTerm(t *testing.T) {
	mit := domain.MatchCandidate{Hits: []domain.MatchHit{{P: 0.99}}}
	ohne := domain.MatchCandidate{}

	sMit := domain.ScoreCandidate(mit, 1)
	sOhne := domain.ScoreCandidate(ohne, 1)

	if sMit <= sOhne {
		t.Errorf("Treffer (%.3f) muss besser bewertet sein als kein Treffer (%.3f)", sMit, sOhne)
	}
	if want := math.Log(domain.MatchMiss); math.Abs(sOhne-want) > 1e-9 {
		t.Errorf("ohne Treffer = %.5f, erwartet log(MISS) = %.5f", sOhne, want)
	}
}

// Dieselbe Art zweimal in derselben Rolle gibt es im Index zwei Mal
// (R1N/wcvp:concept:2570774). Sie darf den Score nicht doppelt heben.
func TestScoreCandidate_ZaehltJedeArtNurEinmal(t *testing.T) {
	einmal := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.9}}}
	doppelt := domain.MatchCandidate{Hits: []domain.MatchHit{
		{ConceptID: "c1", P: 0.9}, {ConceptID: "c1", P: 0.3},
	}}

	if a, b := domain.ScoreCandidate(einmal, 1), domain.ScoreCandidate(doppelt, 1); math.Abs(a-b) > 1e-9 {
		t.Errorf("doppelte Zeile derselben Art aendert den Score: %.5f vs %.5f", a, b)
	}
}

// Von zwei Zeilen derselben Art gilt die hoechste Wahrscheinlichkeit: die Art
// IST da, und die guenstigste Rolle beschreibt das am besten.
func TestScoreCandidate_NimmtDieHoechsteWahrscheinlichkeitJeArt(t *testing.T) {
	c := domain.MatchCandidate{Hits: []domain.MatchHit{
		{ConceptID: "c1", P: 0.2}, {ConceptID: "c1", P: 0.8},
	}}
	want := math.Log(0.8)
	if got := domain.ScoreCandidate(c, 1); math.Abs(got-want) > 1e-9 {
		t.Errorf("Score = %.5f, erwartet log(0.8) = %.5f", got, want)
	}
}

// Der Treuegrad hebt eine Kennart ueber eine Begleitart gleicher Stetigkeit.
func TestScoreCandidate_TreuegradHebtDieKennart(t *testing.T) {
	ohne := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.5}}}
	mit := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.5, Fidelity: 40}}}

	if domain.ScoreCandidate(mit, 1) <= domain.ScoreCandidate(ohne, 1) {
		t.Error("Fidelity erhoeht den Score nicht")
	}
}

// Ein Gebiet, in dem die Arten des Typs kaum vorkommen, drueckt ihn nach
// hinten — aber es schliesst ihn nicht aus.
func TestScoreCandidate_GebietsabdeckungDruecktOhneAuszuschliessen(t *testing.T) {
	gut := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.9}}, AreaCoverage: 0.9, HasArea: true}
	schlecht := domain.MatchCandidate{Hits: []domain.MatchHit{{ConceptID: "c1", P: 0.9}}, AreaCoverage: 0.01, HasArea: true}

	s := domain.ScoreCandidate(schlecht, 1)
	if s >= domain.ScoreCandidate(gut, 1) {
		t.Error("geringe Gebietsabdeckung senkt den Score nicht")
	}
	if math.IsInf(s, -1) || math.IsNaN(s) {
		t.Errorf("Score = %v; eine Abdeckung von 0 darf nicht ausschliessen", s)
	}
}

// Die Spec addiert beide Terme unabhaengig: Summe log P plus W_FID mal Summe
// fidelity. Fidelity traegt aber nur die diagnostic-Zeile, constancy nur die
// constant-Zeile — nimmt man beides aus DERSELBEN Zeile, faellt der Treuegrad
// genau dort weg, wo er am staerksten waere. Gemessen 764 betroffene Paare,
// mittlere verworfene Fidelity 32,7, maximal 99,7.
func TestScoreCandidate_TreuegradUeberlebtDieHoehereStetigkeit(t *testing.T) {
	// Habitat A fuehrt die Art als constant (0.40) UND als diagnostic (phi 45).
	a := domain.MatchCandidate{Hits: []domain.MatchHit{
		{ConceptID: "c1", P: 0.40},
		{ConceptID: "c1", P: domain.MatchDefaultP, Fidelity: 45},
	}}
	// Habitat B fuehrt sie nur als diagnostic.
	b := domain.MatchCandidate{Hits: []domain.MatchHit{
		{ConceptID: "c1", P: domain.MatchDefaultP, Fidelity: 45},
	}}

	if domain.ScoreCandidate(a, 1) <= domain.ScoreCandidate(b, 1) {
		t.Errorf("A (%.3f) ist nicht besser als B (%.3f), obwohl A strikt mehr Evidenz traegt: "+
			"dieselbe Kennart plus hoehere Stetigkeit",
			domain.ScoreCandidate(a, 1), domain.ScoreCandidate(b, 1))
	}
}

// Die Summe laeuft ueber eine Map, und Go iteriert Maps in zufaelliger
// Reihenfolge. Float-Addition ist nicht assoziativ: dieselbe Eingabe kann
// dadurch um einige ULP verschiedene Scores ergeben, und der Gleichstands-
// Tiebreak weiter oben greift dann nicht mehr. Bei vielen Arten ist das
// messbar.
func TestScoreCandidate_IstBitgleichBeiWiederholung(t *testing.T) {
	hits := make([]domain.MatchHit, 0, 40)
	for i := 0; i < 40; i++ {
		hits = append(hits, domain.MatchHit{
			ConceptID: string(rune('a'+i%26)) + string(rune('0'+i/26)),
			P:         0.1 + float64(i)*0.02,
			Fidelity:  float64(i) * 2.3,
		})
	}
	c := domain.MatchCandidate{Hits: hits}

	erste := domain.ScoreCandidate(c, 40)
	for lauf := 0; lauf < 200; lauf++ {
		if got := domain.ScoreCandidate(c, 40); got != erste {
			t.Fatalf("Lauf %d ergab %v, der erste %v — die Summenreihenfolge ist nicht festgelegt",
				lauf, got, erste)
		}
	}
}
