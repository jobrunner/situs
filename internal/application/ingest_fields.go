package application

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Wie wird ein CSV-Feld zu einem Go-Wert? Das ist unabhaengig davon, welche
// Entitaet gerade eingelesen wird, und steht deshalb fuer sich. Alle drei
// behandeln das leere Feld als "keine Angabe" (nil), nicht als Nullwert — der
// Unterschied traegt durch den ganzen Dienst.

func parseOptionalInt(s string) (*int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func parseOptionalBool(s string) (*bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	b := n != 0
	return &b, nil
}

// parseOptionalFloat mirrors parseOptionalInt: an empty fidelity/constancy
// column is absence of data, never a zero value.
func parseOptionalFloat(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	// strconv.ParseFloat nimmt "NaN", "Inf" und "Infinity" an. Ein solcher
	// Messwert ist ein Datenfehler und gehoert hier zurueckgewiesen, nicht
	// erst beim Ausliefern: ein NaN im Score macht die Antwort
	// unserialisierbar, und zwar nachdem der Statuscode 200 schon geschrieben
	// ist — der Aufrufer bekaeme einen abgebrochenen Rumpf ohne Fehlermeldung.
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, fmt.Errorf("%q is not a finite measurement", s)
	}
	return &f, nil
}
