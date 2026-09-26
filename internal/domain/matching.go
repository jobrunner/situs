package domain

import (
	"math"
	"sort"
)

// Die vier Parameter des Matchings. Gesetzt, nicht gelernt — deshalb als
// benannte Konstanten, damit eine spaetere Kalibrierung an echten Aufnahmen
// eine Aenderung an einer Stelle ist. Begruendung je Wert in
// docs/superpowers/specs/2026-09-26-habitat-matching-design.md.
const (
	// MatchMiss ist P(Art | Habitat) fuer eine Art, die der Typ nicht fuehrt.
	// log(0.02) ~= -3.9: spuerbar, aber nicht ausschliessend. Das ist der Kern
	// des Verfahrens — ein harter UND-Filter verliert bei einer einzigen
	// Stoerart in 96 % der Faelle den richtigen Typ.
	MatchMiss = 0.02
	// MatchDefaultP gilt fuer Zeilen, die nur als diagnostic gefuehrt werden
	// und deshalb fidelity statt constancy tragen.
	MatchDefaultP = 0.35
	// MatchFidelityWeight hebt eine Kennart ueber eine Begleitart.
	MatchFidelityWeight = 0.012
	// MatchAreaWeight ist stark genug, um geografisch Unmoegliches zu
	// verdraengen, und zu schwach, um Plausibles zu unterdruecken.
	MatchAreaWeight = 3.0
	// matchAreaFloor haelt log() von der Null fern.
	matchAreaFloor = 0.02
)

// MatchHit ist eine Artenzeile des Kandidaten, die zur Eingabe passt.
type MatchHit struct {
	ConceptID string
	P         float64 // P(Art | Habitat)
	Fidelity  float64 // 0, wenn die Zeile keinen Treuegrad fuehrt
}

// MatchCandidate ist ein Habitattyp mit seinen Treffern zur Eingabe.
type MatchCandidate struct {
	Key          HabitatTypeKey
	Hits         []MatchHit
	AreaCoverage float64
	HasArea      bool
}

// ScoreCandidate berechnet den Log-Likelihood. inputCount ist die Zahl der
// bekannten Eingabe-Arten; jede, die der Typ nicht fuehrt, kostet log(MISS).
func ScoreCandidate(c MatchCandidate, inputCount int) float64 {
	// Je Art gilt die hoechste Wahrscheinlichkeit — die Art IST da, und die
	// guenstigste Rolle beschreibt das am besten. Der Treuegrad wird DAVON
	// GETRENNT maximiert: fidelity traegt nur die diagnostic-Zeile, constancy
	// nur die constant-Zeile. Naehme man beides aus derselben Zeile, fiele der
	// Treuegrad genau dort weg, wo er am staerksten ist — bei einer Art, die
	// ein Habitat als Kennart UND mit hoher Stetigkeit fuehrt.
	//
	// Die Trennung erledigt zugleich die Doppelzeilen, die der Index in zwei
	// Faellen traegt (zwei Namen auf einer Konzept-ID): sie heben den Score
	// nicht doppelt.
	bestP := map[string]float64{}
	maxFid := map[string]float64{}
	for _, h := range c.Hits {
		if p, ok := bestP[h.ConceptID]; !ok || h.P > p {
			bestP[h.ConceptID] = h.P
		}
		if f, ok := maxFid[h.ConceptID]; !ok || h.Fidelity > f {
			maxFid[h.ConceptID] = h.Fidelity
		}
	}

	// In sortierter Reihenfolge summieren, nicht in der einer Map: Go iteriert
	// Maps zufaellig, und Float-Addition ist nicht assoziativ. Ohne das ergibt
	// dieselbe Eingabe um einige ULP verschiedene Scores, und der
	// Gleichstands-Tiebreak in der Anwendungsschicht greift nicht mehr.
	ids := make([]string, 0, len(bestP))
	for id := range bestP {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	score := 0.0
	for _, id := range ids {
		score += math.Log(bestP[id])
		score += MatchFidelityWeight * maxFid[id]
	}
	for i := len(bestP); i < inputCount; i++ {
		score += math.Log(MatchMiss)
	}
	if c.HasArea {
		score += MatchAreaWeight * math.Log(math.Max(c.AreaCoverage, matchAreaFloor))
	}
	return score
}
