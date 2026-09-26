package application

import (
	"math"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// Die Belege einer Antwort: welche Art hat fuer diesen Habitattyp gesprochen,
// und mit welcher Kennzahl? Das ist ein anderes Thema als die Rangfolge selbst
// — dort wird geordnet, hier wird erklaert, warum.

// verdichteBeleg haelt je Art genau einen Beleg fest: die Zeile, die der Score
// genutzt hat (die hoechste Wahrscheinlichkeit), ergaenzt um den hoechsten
// Treuegrad derselben Art. So weist die Antwort genau die Evidenz aus, die in
// die Rangfolge einging — species und matched zaehlen dasselbe.
func verdichteBeleg(belege map[domain.HabitatTypeKey][]input.MatchSpecies,
	key domain.HabitatTypeKey, conceptID string, r domain.SpeciesRole) {
	neu := input.MatchSpecies{
		ConceptID: conceptID, Role: r.Role, Constancy: r.Constancy, Fidelity: r.Fidelity,
	}
	for i, alt := range belege[key] {
		if alt.ConceptID != conceptID {
			continue
		}
		// Die Rolle mit der hoeheren Wahrscheinlichkeit gewinnt; der
		// Treuegrad wird davon getrennt uebernommen, genau wie im Score.
		if rollenWahrscheinlichkeit(r) > wahrscheinlichkeitVon(alt) {
			neu.Fidelity = hoehere(alt.Fidelity, r.Fidelity)
			belege[key][i] = neu
			return
		}
		belege[key][i].Fidelity = hoehere(alt.Fidelity, r.Fidelity)
		return
	}
	belege[key] = append(belege[key], neu)
}

// wahrscheinlichkeitVon rechnet einen bereits festgehaltenen Beleg auf
// dieselbe Skala wie rollenWahrscheinlichkeit zurueck.
func wahrscheinlichkeitVon(s input.MatchSpecies) float64 {
	return rollenWahrscheinlichkeit(domain.SpeciesRole{Constancy: s.Constancy})
}

// hoehere waehlt den groesseren von zwei optionalen Werten. Das Fehlen ist
// "kein Wert", nicht null — sonst schluege eine fehlende Angabe eine
// vorhandene. Der Vergleich selbst laeuft ueber math.Max, damit aus ">" gegen
// ">=" kein Unterschied wird, den kein Test je sehen koennte.
func hoehere(a, b *float64) *float64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	groesser := math.Max(*a, *b)
	return &groesser
}

// rollenWahrscheinlichkeit liest constancy als P(Art | Habitat). Zeilen ohne
// Stetigkeit — die nur als diagnostic gefuehrten und alle aus Aggregaten
// abgeleiteten — bekommen den Vorgabewert.
func rollenWahrscheinlichkeit(r domain.SpeciesRole) float64 {
	// nil und 0 sind zwei verschiedene Aussagen. Keine Angabe heisst "fuer
	// diese Zeile wurde keine Stetigkeit gemessen" (so bei den nur als
	// diagnostic gefuehrten und allen aus Aggregaten abgeleiteten Zeilen) und
	// bekommt den Vorgabewert. Eine ausdrueckliche 0 heisst "kommt in keiner
	// Aufnahme dieses Typs vor" — sie mit dem Vorgabewert zu belegen machte
	// aus einer Nicht-Vorkommen-Zeile einen positiven Treffer.
	if r.Constancy == nil {
		return domain.MatchDefaultP
	}
	// Ein negativer Wert ist ein Datenfehler; er wird auf den MISS-Wert
	// gelegt, nicht auf den Vorgabewert, damit er nicht wie fehlende Auskunft
	// wirkt. Dasselbe gilt fuer die ausdrueckliche Null, deren log sonst
	// minus unendlich waere.
	if *r.Constancy <= 0 {
		return domain.MatchMiss
	}
	// Eine Stetigkeit von 100 heisst "in jeder Aufnahme dieses Typs", also
	// P = 1,0. Sie auf 0,99 zu kappen machte sie von echten 99 ununterscheidbar
	// und verschoebe die Rangfolge. Nur Werte ueber 100 werden begrenzt — die
	// waeren ein Datenfehler, und log(>1) gaebe einen Bonus fuer einen kaputten
	// Wert.
	return math.Min(*r.Constancy/100, 1.0)
}

func wert(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
