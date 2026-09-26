package domain

import "math"

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
	// Je Art gilt die hoechste Wahrscheinlichkeit: die Art IST da, und die
	// guenstigste Rolle beschreibt das am besten. Zugleich verhindert das,
	// dass die zwei bekannten Doppelzeilen den Score doppelt heben.
	best := make(map[string]MatchHit, len(c.Hits))
	for _, h := range c.Hits {
		if cur, ok := best[h.ConceptID]; !ok || h.P > cur.P {
			best[h.ConceptID] = h
		}
	}

	score := 0.0
	for _, h := range best {
		score += math.Log(h.P)
		score += MatchFidelityWeight * h.Fidelity
	}
	for i := len(best); i < inputCount; i++ {
		score += math.Log(MatchMiss)
	}
	if c.HasArea {
		score += MatchAreaWeight * math.Log(math.Max(c.AreaCoverage, matchAreaFloor))
	}
	return score
}
