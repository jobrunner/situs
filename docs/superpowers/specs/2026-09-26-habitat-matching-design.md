# Habitat-Matching: Artenliste → Rangliste der Habitattypen

**Status:** Entwurf, zur Umsetzung freigegeben
**Datum:** 2026-09-26
**Revidiert:** die „kein Scoring/Ranking"-Entscheidung aus
`2026-08-18-situs-foundation-design.md`

## Die revidierte Entscheidung

Das Fundament hat Scoring ausdrücklich ausgeschlossen — in `CLAUDE.md`
(„Known ceiling"), in `docs/reference/http-api.md` („Nicht vergessen, sondern
entschieden") und im Untertitel des Explorers („rechnet **nichts selbst
aus**"). Diese Entscheidung wird hiermit **bewusst umgekehrt**, nicht
übersehen.

Der Grund ist eine Messung, keine Bequemlichkeit. Über alle 198 Level-3-
Habitattypen mit Artenliste gilt:

- **62 %** aller 3561 Arten kommen in genau einem Habitattyp vor,
- **53 %** aller Habitatpaare teilen keine einzige Art,
- die häufigste Art überhaupt (*Dactylis glomerata*) erreicht 65 von 198.

Die Trennschärfe liegt also in den Daten selbst. Drei bis fünf Arten genügen
gemessen, um den richtigen Typ auf Rang 1 zu bringen. Das vorwegzunehmen, was
das Fundament offenlassen wollte — eine Gewichtung —, ist hier keine
Geschmacksfrage mehr, sondern eine Auswertung, die jeder Aufrufer sonst selbst
und schlechter nachbauen müsste.

**Was die Umkehr nicht umfasst:** keine Bewertung der *Güte* einer Aufnahme,
keine Empfehlung, keine Schwellenwerte, ab denen ein Treffer „gilt". Der Dienst
ordnet, er entscheidet nicht.

## Die Route

```
POST /v1/habitat-types/match
```

```json
{
  "concept_ids": ["wcvp:concept:83891", "wcvp:concept:2457314"],
  "typology": "eunis@2021",
  "level": 3,
  "area": "GER",
  "limit": 10
}
```

| Feld | Pflicht | Vorgabe | Bedeutung |
|---|---|---|---|
| `concept_ids` | ja | — | die notierten Arten; wie beim bestehenden Batch, **keine** verbatim Namen (die Leseseite bleibt autark) |
| `typology` | nein | `eunis@2021` | |
| `level` | nein | `3` | die Ebene, auf der gerankt wird |
| `area` | nein | — | WGSRPD-Level-3-Code; wirkt als Plausibilitätsterm, **nicht** als Filter |
| `limit` | nein | `10` | Länge der Rangliste, 1–50 |

Antwort:

```json
{
  "input": [
    {"concept_id": "wcvp:concept:83891", "known": true},
    {"concept_id": "gbif:7777", "known": false, "reason": "unknown_backbone"}
  ],
  "matches": [
    {"typology": "eunis@2021", "code": "T17", "name_en": "Fagus forest on non-acid soils",
     "score": -1.30, "matched": 3, "of": 3,
     "species": [{"concept_id": "...", "role": "constant", "constancy": 99}]}
  ]
}
```

**`score` ist ein Log-Likelihood, keine Wahrscheinlichkeit** — ausdrücklich so
entschieden. Ein Posterior in Prozent würde eine Genauigkeit behaupten, die die
Daten nicht tragen: die Parameter sind gesetzt und nicht kalibriert, und die
Unabhängigkeitsannahme des naiven Bayes ist bei gemeinsam auftretenden Arten
falsch. Nur die **Reihenfolge** ist belastbar, und `matched`/`of` macht
nachvollziehbar, worauf sie beruht.

`input` spiegelt jede Eingabe zurück, mit `known` und bei `false` demselben
`reason`-Vokabular wie `POST /v1/species/habitat-types`
(`unknown_backbone` / `unknown_concept`). Eine unbekannte ID darf eine Anfrage
über 300 nicht scheitern lassen.

## Der Algorithmus

Kern ist eine Umdeutung der vorhandenen Daten: **`constancy` ist bereits eine
Wahrscheinlichkeit.** `constancy = 99` heißt, die Art kommt in 99 % der
Aufnahmen dieses Typs vor, also `P(Art | Habitat) = 0,99`.

```
score(H) = Σ log P(a | H)                     über alle bekannten Eingabe-Arten
         + W_FID · Σ fidelity(a, H)           Treuegrad als Zuschlag
         + W_AREA · log(Gebietsabdeckung(H))  nur wenn area gesetzt ist

P(a | H) = constancy/100   wenn die Zeile eine Stetigkeit führt
         = DEFAULT_P       wenn die Art dort nur als diagnostic geführt wird
         = MISS            wenn das Habitat die Art nicht führt
```

| Parameter | Wert | Begründung |
|---|---|---|
| `MISS` | `0.02` | Eine nicht geführte Art kostet `log(0,02) ≈ −3,9` — spürbar, aber nicht ausschließend. **Das ist der Kern des Entwurfs**, siehe unten. |
| `DEFAULT_P` | `0.35` | `diagnostic`-Zeilen führen `fidelity` statt `constancy`; 0,35 hat sich im Durchlauf bewährt. |
| `W_FID` | `0.012` | Eine Kennart wiegt schwerer als eine Begleitart. |
| `W_AREA` | `3.0` | Stark genug, um geografisch Unmögliches zu verdrängen, zu schwach, um Plausibles zu unterdrücken. |

Die Gebietsabdeckung ist der Anteil der Habitat-Arten, die im angefragten
Gebiet verbreitet sind (`species_distribution`), mit `max(anteil, 0.02)` gegen
`log(0)`.

**Alle vier Werte sind gesetzt, nicht gelernt.** Sie gehören als benannte
Konstanten in den Code, nicht als Literale in eine Formel, damit eine spätere
Kalibrierung an echten Aufnahmen eine Änderung an einer Stelle ist.

## Warum kein Filter — der wichtigste Befund

Der naheliegende Entwurf wäre ein Filter: behalte die Habitate, die alle
genannten Arten führen. **Gemessen ist er unbrauchbar.** Enthält die Eingabe
eine einzige Art, die das wahre Habitat nicht führt — eine Begleitart, eine
Ruderale, eine Fehlbestimmung —, fällt das wahre Habitat in **96 bis 97 %**
der Fälle heraus, und die Antwort ist meist leer.

Derselbe Fall mit dem Likelihood-Ansatz oben: zwei Störarten kosten **zwei
Prozentpunkte**. Das ist nicht dieselbe Methode mit anderen Parametern,
sondern der Unterschied zwischen brauchbar und unbrauchbar. Der `MISS`-Term
ist deshalb keine Feinheit, sondern der Entwurf.

## Abnahmekriterien

Gemessen an simulierten Aufnahmen (Arten nach ihrer Stetigkeit gezogen, wie im
Feld), 198 Habitate, 40 Ziehungen je Habitat. Eine Implementierung, die diese
Werte deutlich verfehlt, weicht vom Entwurf ab:

| Eingabe-Arten | Störarten | Rang 1 | Top 3 | Top 5 |
|---|---|---|---|---|
| 3 | 0 | 82 % | 96 % | 99 % |
| 3 | 2 | 80 % | 95 % | 99 % |
| 4 | 0 | 87 % | 98 % | 100 % |
| 5 | 0 | 90 % | 99 % | 100 % |
| 5 | 2 | 90 % | 99 % | 100 % |

Zusätzlich als Festlegung:

- `Fagus sylvatica` + `Anemone nemorosa` + `Galium odoratum` → **T17** auf Rang 1
  (Waldmeister-Buchenwald), mit Abstand zum zweiten.
- Mit `area=GER` erscheint **kein** makaronesisches Habitat unter den ersten
  drei. Ohne `area` geschah das in 38 von 4470 simulierten Aufnahmen.

## Das Gebiet ist Plausibilitätsprüfung, nicht Filter

Gemessen verbessert `area` die Trefferquote **nicht** (Top 3: 95 % ohne, 95 %
mit) — die Artenliste trägt die geografische Information bereits. Sein Nutzen
ist ein anderer und eindeutig: es verhindert unmögliche Vorschläge. Deshalb
geht es als Term in den Score und **nicht** als Ausschlusskriterium: ein
Habitat, dessen Arten im Gebiet selten sind, rutscht nach hinten, verschwindet
aber nicht. Als harter Vorfilter taugt es ohnehin kaum — für Deutschland
blieben bei 50 % Abdeckung noch 149 der 198 Typen übrig.

## Grenzen, die in die Antwort gehören

- **Formation U (Fels und Geröll) führt keine einzige Kennart** — alle 36
  Level-3-Typen. Formation V nur 12 von 31. Über Artenlisten sind damit 198
  von 270 Level-3-Typen erreichbar, und die Lücke trifft gezielt die
  vegetationsarmen Standorte. Die Route kann diese Typen nie vorschlagen; das
  gehört dokumentiert, nicht kaschiert.
- Die Route beantwortet **nicht**, ob eine Aufnahme überhaupt in diese
  Typologie fällt.
- Sie liefert keine Aussage über Deckung, Schichtung oder Standort.

## Nicht Teil dieser Spec

- Kalibrierung der Parameter an echten Aufnahmen.
- Ein adaptiver Bestimmungsschlüssel („welche Art als nächste?"). Gemessen ist
  er dem freien Notieren **dramatisch unterlegen** — 0 % gegen 83 % nach drei
  Schritten —, weil eine frei notierte Art bis zu `log(198) ≈ 5,3 nat` liefert,
  eine Ja/Nein-Frage aber höchstens `log(2) ≈ 0,69 nat`. Nicht vergessen,
  sondern gemessen und verworfen.
- Ein Ranking über mehrere Typologien hinweg.

## Was mitgeändert werden muss

Die Umkehr ist erst vollständig, wenn die drei Stellen, die das Gegenteil
sagen, nachgezogen sind:

1. `CLAUDE.md`, „Known ceiling" — Scoring ist nicht mehr ausgeschlossen.
2. `docs/reference/http-api.md` — der Absatz zu den Ko-Kennarten begründet das
   Fehlen eines zusammenfassenden Endpunkts.
3. `internal/adapters/http/explorer.html` — der Untertitel „Es wird nichts
   berechnet oder zusammengefasst, was der Dienst nicht selbst liefert" bleibt
   für die übrigen Panels richtig, muss aber für das neue Panel eingeschränkt
   werden.

Dazu die üblichen: `openapi.yaml` in beiden Kopien, der Routen-Vertragstest,
`docs/reference/measured-index.md` für die Kennzahlen dieser Spec.
