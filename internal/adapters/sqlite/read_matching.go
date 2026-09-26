package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
)

// HabitatAreaCoverage liefert je Schluessel den Anteil der Arten, die im
// Gebiet verbreitet sind. Typen, zu denen keine einzige Art eine
// Verbreitungsangabe traegt, fehlen im Ergebnis: unbeurteilbar ist nicht
// dasselbe wie unplausibel.
//
// Eine Abfrage fuer alle Kandidaten statt einer je Kandidat. Die
// Platzhalterliste wird aus der ANZAHL der Schluessel gebaut, nie aus ihren
// Werten — die bleiben gebundene Parameter (gosec G201).
//
// Gezaehlt werden KONZEPTE, nicht Zeilen: dieselbe Art steht in demselben
// Habitattyp regelmaessig als constant und diagnostic und dominant. Zaehlte
// der Zaehler Zeilen gegen den Konzept-Nenner, waere die "Abdeckung" groesser
// als 1, und aus dem Malus wuerde ein Bonus — der Gebietsterm mueesse dann die
// Zeilenredundanz der Quelle statt der Geografie.
//
// Schemaparametrisiert, wie CLAUDE.md es fuer jede Gebietsabfrage verlangt:
// heute traegt species_distribution nur wgsrpd_l3, aber ein zweites Schema
// wuerde sonst stumm mitaddiert.
func (d *DB) HabitatAreaCoverage(ctx context.Context, keys []domain.HabitatTypeKey, areaCode string) (map[domain.HabitatTypeKey]float64, error) {
	out := map[domain.HabitatTypeKey]float64{}
	if len(keys) == 0 || areaCode == "" {
		return out, nil
	}

	args := make([]any, 0, len(keys)*2+3)
	args = append(args, domain.SchemeWGSRPDL3, areaCode)
	for _, k := range keys {
		args = append(args, string(k.Typology), k.Code)
	}
	args = append(args, domain.SchemeWGSRPDL3)
	pairs := strings.TrimSuffix(strings.Repeat("(?,?),", len(keys)), ",")

	rows, err := d.QueryContext(ctx, `
		SELECT s.typology_id, s.code,
		       CAST(COUNT(DISTINCT CASE WHEN d.area_code IS NOT NULL
		                                THEN s.concept_id END) AS REAL)
		         / COUNT(DISTINCT s.concept_id) AS coverage
		FROM species_role s
		LEFT JOIN species_distribution d
		       ON d.concept_id = s.concept_id
		      AND d.area_scheme = ?
		      AND d.area_code = ?
		WHERE s.concept_id IS NOT NULL AND s.concept_id <> ''
		  AND (s.typology_id, s.code) IN (VALUES `+pairs+`)
		  AND EXISTS (SELECT 1 FROM species_distribution x
		               WHERE x.concept_id = s.concept_id AND x.area_scheme = ?)
		GROUP BY s.typology_id, s.code`, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying area coverage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var typology, code string
		var cov float64
		if err := rows.Scan(&typology, &code, &cov); err != nil {
			return nil, fmt.Errorf("sqlite: scanning area coverage: %w", err)
		}
		out[domain.HabitatTypeKey{Typology: domain.TypologyID(typology), Code: code}] = cov
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: reading area coverage: %w", err)
	}
	return out, nil
}
