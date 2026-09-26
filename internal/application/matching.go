package application

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// MatchHabitatTypes ordnet Habitattypen danach, wie gut sie eine beobachtete
// Artenliste erklaeren.
//
// Kandidat ist nur, wer mindestens eine Eingabe-Art fuehrt: jeder andere Typ
// traegt den identischen Score k*log(MISS) und ist damit rangneutral. Das
// erspart den vollen Durchlauf ueber alle Typen der Ebene.
func (q *QueryService) MatchHabitatTypes(ctx context.Context, req input.MatchRequest) (input.MatchResult, error) {
	res := input.MatchResult{Input: make([]input.MatchInput, 0, len(req.ConceptIDs)), Matches: []input.MatchEntry{}}

	// Das Gebiet wird zuerst geprueft, vor jeder Artenabfrage: ein unbekannter
	// Code ist ein Fehler des Aufrufers, und das haengt nicht davon ab, ob die
	// Artenliste am Ende Kandidaten ergibt. Dieselbe Pruefung wie beim
	// vorhandenen ?area= (area.go) — sie liegt hier und nicht im Handler, weil
	// hier der Repository-Zugang ist.
	if req.Area != "" {
		known, err := q.repo.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3)
		if err != nil {
			return input.MatchResult{}, err
		}
		if !slices.Contains(known, req.Area) {
			return input.MatchResult{}, fmt.Errorf("area %q: %w", req.Area, input.ErrUnknownArea)
		}
	}

	hits := map[domain.HabitatTypeKey][]domain.MatchHit{}
	bekannt := 0
	for _, id := range req.ConceptIDs {
		entry := input.MatchInput{ConceptID: id}
		if !strings.HasPrefix(id, indexBackbone+":") { // dieselbe Pruefung wie der Batch, query.go:329
			entry.Reason = input.ReasonUnknownBackbone
			res.Input = append(res.Input, entry)
			continue
		}
		rollen, err := q.repo.SpeciesRolesByConcept(ctx, id)
		if err != nil {
			return input.MatchResult{}, fmt.Errorf("matching %q: %w", id, err)
		}
		if len(rollen) == 0 {
			entry.Reason = input.ReasonUnknownConcept
			res.Input = append(res.Input, entry)
			continue
		}
		entry.Known = true
		bekannt++
		res.Input = append(res.Input, entry)

		for _, r := range rollen {
			if string(r.Key.Typology) != req.Typology {
				continue
			}
			hits[r.Key] = append(hits[r.Key], domain.MatchHit{
				ConceptID: id, P: rollenWahrscheinlichkeit(r), Fidelity: wert(r.Fidelity),
			})
		}
	}
	if bekannt == 0 || len(hits) == 0 {
		return res, nil
	}

	keys := make([]domain.HabitatTypeKey, 0, len(hits))
	for k := range hits {
		keys = append(keys, k)
	}
	abdeckung := map[domain.HabitatTypeKey]float64{}
	if req.Area != "" {
		var err error
		if abdeckung, err = q.repo.HabitatAreaCoverage(ctx, keys, req.Area); err != nil {
			return input.MatchResult{}, fmt.Errorf("matching area coverage: %w", err)
		}
	}

	for _, k := range keys {
		ht, err := q.repo.HabitatType(ctx, k)
		if err != nil {
			return input.MatchResult{}, fmt.Errorf("matching habitat type %s: %w", k, err)
		}
		// Level ist ein Zeiger: nil heisst "die Ebene ist unbekannt", und ein
		// Typ ohne Ebenenangabe wird nicht weggefiltert — die Auskunft fehlt,
		// sie widerspricht nicht.
		if req.Level > 0 && ht.Level != nil && *ht.Level != req.Level {
			continue
		}
		cov, hat := abdeckung[k]
		c := domain.MatchCandidate{Key: k, Hits: hits[k], AreaCoverage: cov, HasArea: hat}
		gezaehlt := map[string]struct{}{}
		for _, h := range hits[k] {
			gezaehlt[h.ConceptID] = struct{}{}
		}
		res.Matches = append(res.Matches, input.MatchEntry{
			Typology: string(k.Typology), Code: k.Code, NameEN: ht.NameEN,
			Score: domain.ScoreCandidate(c, bekannt), Matched: len(gezaehlt), Of: bekannt,
		})
	}

	sort.Slice(res.Matches, func(i, j int) bool {
		if res.Matches[i].Score != res.Matches[j].Score {
			return res.Matches[i].Score > res.Matches[j].Score
		}
		return res.Matches[i].Code < res.Matches[j].Code // stabil bei Gleichstand
	})
	if req.Limit > 0 && len(res.Matches) > req.Limit {
		res.Matches = res.Matches[:req.Limit]
	}
	return res, nil
}

// rollenWahrscheinlichkeit liest constancy als P(Art | Habitat). Zeilen ohne
// Stetigkeit — die nur als diagnostic gefuehrten und alle aus Aggregaten
// abgeleiteten — bekommen den Vorgabewert.
func rollenWahrscheinlichkeit(r domain.SpeciesRole) float64 {
	if r.Constancy != nil && *r.Constancy > 0 {
		return math.Min(*r.Constancy/100, 0.99)
	}
	return domain.MatchDefaultP
}

func wert(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
