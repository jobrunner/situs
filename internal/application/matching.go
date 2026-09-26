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
	// Das Gebiet wird zuerst geprueft, vor jeder Artenabfrage: ein unbekannter
	// Code ist ein Fehler des Aufrufers, und das haengt nicht davon ab, ob die
	// Artenliste am Ende Kandidaten ergibt. Dieselbe Pruefung wie beim
	// vorhandenen ?area= (area.go) — sie liegt hier und nicht im Handler, weil
	// hier der Repository-Zugang ist.
	if err := q.pruefeTypologie(ctx, req.Typology); err != nil {
		return input.MatchResult{}, err
	}
	if err := q.pruefeGebiet(ctx, req.Area); err != nil {
		return input.MatchResult{}, err
	}

	res, hits, belege, bekannt, err := q.sammleTreffer(ctx, req)
	if err != nil {
		return input.MatchResult{}, err
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
		if abdeckung, err = q.repo.HabitatAreaCoverage(ctx, keys, req.Area); err != nil {
			return input.MatchResult{}, fmt.Errorf("matching area coverage: %w", err)
		}
	}

	if res.Matches, err = q.ordneKandidaten(ctx, req, keys, hits, belege, abdeckung, bekannt); err != nil {
		return input.MatchResult{}, err
	}
	return res, nil
}

// pruefeTypologie weist eine Typologie zurueck, die der Index nicht fuehrt.
// Ohne das waere ein Tippfehler in typology nicht von "diese Arten passen
// nirgends" zu unterscheiden — dieselbe Ueberlegung wie beim Gebiet.
func (q *QueryService) pruefeTypologie(ctx context.Context, typology string) error {
	if typology == "" {
		return nil
	}
	bekannt, err := q.repo.Typologies(ctx)
	if err != nil {
		return err
	}
	for _, t := range bekannt {
		if string(t.Typology.ID) == typology {
			return nil
		}
	}
	return fmt.Errorf("typology %q: %w", typology, input.ErrUnknownTypology)
}

// pruefeGebiet weist einen Gebietscode zurueck, den der Index nicht kennt.
func (q *QueryService) pruefeGebiet(ctx context.Context, area string) error {
	if area == "" {
		return nil
	}
	known, err := q.repo.KnownAreaCodes(ctx, domain.SchemeWGSRPDL3)
	if err != nil {
		return err
	}
	if !slices.Contains(known, area) {
		return fmt.Errorf("area %q: %w", area, input.ErrUnknownArea)
	}
	return nil
}

// sammleTreffer loest jede Eingabe-ID auf und sammelt je Habitattyp die
// passenden Artenzeilen. Es spiegelt dabei jede Eingabe zurueck — auch die
// unbekannten, mit ihrem Grund.
func (q *QueryService) sammleTreffer(ctx context.Context, req input.MatchRequest) (
	input.MatchResult, map[domain.HabitatTypeKey][]domain.MatchHit,
	map[domain.HabitatTypeKey][]input.MatchSpecies, int, error) {
	res := input.MatchResult{Input: make([]input.MatchInput, 0, len(req.ConceptIDs)), Matches: []input.MatchEntry{}}
	hits := map[domain.HabitatTypeKey][]domain.MatchHit{}
	belege := map[domain.HabitatTypeKey][]input.MatchSpecies{}
	bekannt := 0
	gesehen := map[string]struct{}{}

	for _, id := range req.ConceptIDs {
		entry := input.MatchInput{ConceptID: id}
		if !strings.HasPrefix(id, indexBackbone+":") { // dieselbe Pruefung wie der Batch, query.go:329
			entry.Reason = input.ReasonUnknownBackbone
			res.Input = append(res.Input, entry)
			continue
		}
		rollen, err := q.repo.SpeciesRolesByConcept(ctx, id)
		if err != nil {
			return input.MatchResult{}, nil, nil, 0, fmt.Errorf("matching %q: %w", id, err)
		}
		if len(rollen) == 0 {
			entry.Reason = input.ReasonUnknownConcept
			res.Input = append(res.Input, entry)
			continue
		}
		entry.Known = true
		res.Input = append(res.Input, entry)
		// Eine doppelt genannte Art zaehlt einmal: sonst kostete die Dublette
		// einen MISS, und matched/of — die Zahl, mit der die Antwort ihre
		// Reihenfolge begruendet — waere falsch. Zurueckgespiegelt wird die
		// Eingabe trotzdem vollstaendig, wie beim Batch.
		if _, doppelt := gesehen[id]; doppelt {
			continue
		}
		gesehen[id] = struct{}{}
		bekannt++

		for _, r := range rollen {
			if string(r.Key.Typology) != req.Typology {
				continue
			}
			hits[r.Key] = append(hits[r.Key], domain.MatchHit{
				ConceptID: id, P: rollenWahrscheinlichkeit(r), Fidelity: wert(r.Fidelity),
			})
			belege[r.Key] = append(belege[r.Key], input.MatchSpecies{
				ConceptID: id, Role: r.Role, Constancy: r.Constancy, Fidelity: r.Fidelity,
			})
		}
	}
	return res, hits, belege, bekannt, nil
}

// ordneKandidaten bewertet jeden Kandidaten und sortiert absteigend.
func (q *QueryService) ordneKandidaten(ctx context.Context, req input.MatchRequest,
	keys []domain.HabitatTypeKey, hits map[domain.HabitatTypeKey][]domain.MatchHit,
	belege map[domain.HabitatTypeKey][]input.MatchSpecies,
	abdeckung map[domain.HabitatTypeKey]float64, bekannt int) ([]input.MatchEntry, error) {
	out := []input.MatchEntry{}
	for _, k := range keys {
		ht, err := q.repo.HabitatType(ctx, k)
		if err != nil {
			return nil, fmt.Errorf("matching habitat type %s: %w", k, err)
		}
		// Level ist ein Zeiger: nil heisst "die Ebene ist unbekannt", und ein
		// Typ ohne Ebenenangabe wird nicht weggefiltert — die Auskunft fehlt,
		// sie widerspricht nicht.
		if req.Level > 0 && ht.Level != nil && *ht.Level != req.Level {
			continue
		}
		cov, hat := abdeckung[k]
		gezaehlt := map[string]struct{}{}
		for _, h := range hits[k] {
			gezaehlt[h.ConceptID] = struct{}{}
		}
		out = append(out, input.MatchEntry{
			Typology: string(k.Typology), Code: k.Code, NameEN: ht.NameEN,
			Score: domain.ScoreCandidate(domain.MatchCandidate{
				Key: k, Hits: hits[k], AreaCoverage: cov, HasArea: hat,
			}, bekannt),
			Matched: len(gezaehlt), Of: bekannt, Species: belege[k],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Code < out[j].Code // stabil bei Gleichstand
	})
	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

// rollenWahrscheinlichkeit liest constancy als P(Art | Habitat). Zeilen ohne
// Stetigkeit — die nur als diagnostic gefuehrten und alle aus Aggregaten
// abgeleiteten — bekommen den Vorgabewert.
func rollenWahrscheinlichkeit(r domain.SpeciesRole) float64 {
	if r.Constancy == nil || *r.Constancy <= 0 {
		return domain.MatchDefaultP
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
