package application

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// MatchHabitatTypes ordnet Habitattypen danach, wie gut sie eine beobachtete
// Artenliste erklaeren.
//
// Kandidat ist nur, wer mindestens eine Eingabe-Art fuehrt. Ohne area ist das
// eine reine Ersparnis: jeder andere Typ traegt den identischen Score
// k*log(MISS) und ist rangneutral.
//
// MIT area ist es eine Entscheidung. Ein Typ ohne Treffer, aber mit perfekter
// Gebietsabdeckung haette rechnerisch den besseren Score als ein Kandidat mit
// einem Treffer und schlechter Abdeckung (-11,74 gegen -16,92). Er erscheint
// trotzdem nicht, und das ist fachlich richtig: die Artenliste ist die
// Evidenz, das Gebiet nur ein Korrektiv. Ein Typ, zu dem keine einzige
// notierte Art passt, ist kein Kandidat, so einleuchtend die Geografie auch sein
// mag. Der Score ist damit ein Likelihood ueber die Kandidaten, nicht ueber
// alle Typen der Ebene.
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
	gesehen := map[string]bool{} // ID -> war sie bekannt?

	for _, id := range req.ConceptIDs {
		entry := input.MatchInput{ConceptID: id}
		if !strings.HasPrefix(id, indexBackbone+":") { // dieselbe Pruefung wie der Batch, query.go:329
			entry.Reason = input.ReasonUnknownBackbone
			res.Input = append(res.Input, entry)
			continue
		}
		// Eine doppelt genannte Art wird einmal abgefragt und einmal
		// gezaehlt. Sonst kostete die Dublette einen MISS, und matched/of —
		// die Zahl, mit der die Antwort ihre Reihenfolge begruendet — waere
		// falsch; ausserdem befragte eine Liste mit 300-mal derselben ID den
		// Index 300-mal. Zurueckgespiegelt wird die Eingabe trotzdem
		// vollstaendig, wie beim Batch.
		if bekanntSchon, doppelt := gesehen[id]; doppelt {
			entry.Known = bekanntSchon
			if !bekanntSchon {
				entry.Reason = input.ReasonUnknownConcept
			}
			res.Input = append(res.Input, entry)
			continue
		}

		rollen, err := q.repo.SpeciesRolesByConcept(ctx, id)
		if err != nil {
			return input.MatchResult{}, nil, nil, 0, fmt.Errorf("matching %q: %w", id, err)
		}
		if len(rollen) == 0 {
			gesehen[id] = false
			entry.Reason = input.ReasonUnknownConcept
			res.Input = append(res.Input, entry)
			continue
		}
		gesehen[id] = true
		entry.Known = true
		res.Input = append(res.Input, entry)
		bekannt++

		for _, r := range rollen {
			if string(r.Key.Typology) != req.Typology {
				continue
			}
			hits[r.Key] = append(hits[r.Key], domain.MatchHit{
				ConceptID: id, P: rollenWahrscheinlichkeit(r), Fidelity: wert(r.Fidelity),
			})
			verdichteBeleg(belege, r.Key, id, r)
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
	// cmp.Compare statt handgeschriebener Vergleiche: absteigend nach Score,
	// bei Gleichstand aufsteigend nach Code. Die Drei-Wege-Form kommt ohne
	// ">" aus, dessen Unterschied zu ">=" hier ohnehin keiner waere — der
	// zweite Vergleich wird nur bei gleichem Score erreicht, und zwei
	// Eintraege mit gleichem Code kann es nicht geben.
	slices.SortFunc(out, func(a, b input.MatchEntry) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return strings.Compare(a.Code, b.Code)
	})
	if req.Limit > 0 {
		out = out[:min(len(out), req.Limit)]
	}
	return out, nil
}
