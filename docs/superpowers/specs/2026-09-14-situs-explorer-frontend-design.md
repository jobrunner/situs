# Explorer-Frontend & Zeigerwertanalyse: Design-Spec

Stand: 2026-09-14.

**Ziel:** situs bekommt eine eingebettete, abhängigkeitsfreie Weboberfläche
unter `GET /`, mit der sich jeder Lese-Endpunkt ohne weitere Werkzeuge
ausprobieren lässt — plus die zwei Routen, die dafür fehlen: eine Namenssuche
über die Index-eigenen Namen und eine Zeigerwertanalyse über eine Artenliste.

**Warum überhaupt:** Die Kernfrage des Dienstes (Artenliste → Habitattypen)
ist heute nur über Concept-IDs stellbar (`wcvp:concept:83891`). Die
Namensauflösung leistet hostus — aber ein Demonstrations- und Testwerkzeug,
das eine zweite laufende Instanz voraussetzt, ist kein Testwerkzeug. Der
Index führt jedoch **3780 eigene `verbatim_name`-Einträge** (3314 davon mit
Concept-ID, gemessen an `out/situs-0.3.0.sqlite`): genug, um im Browser
tippen zu können, ohne hostus.

**Abgrenzung — was diese Suche ausdrücklich NICHT ist:** kein hostus-Ersatz.
Keine Fuzzy-Auflösung, keine Synonyme, keine Autorenvarianten, keine
Homonym-Logik, kein Backbone-Wissen. Sie beantwortet ausschließlich „welche
Namen führt *dieser* Index, und unter welcher Concept-ID". Wer echte
Namensauflösung braucht, ruft hostus — daran ändert dieses Spec nichts.

## 1. Was dazukommt

| Route | Methode | Zweck |
|---|---|---|
| `/` | GET | Explorer-Seite, als HTML ins Binary eingebettet |
| `/v1/species/search` | GET | Namenssuche über die Index-eigenen `verbatim_name` |
| `/v1/species/traits/summary` | POST | Zeigerwertanalyse über eine Artenliste |

Alle drei sind in `internal/adapters/http/openapi.yaml` zu dokumentieren und
werden vom bestehenden `TestRoutesMatchOpenAPISpec` in beide Richtungen
geprüft. Es kommt **keine neue Go-Abhängigkeit** hinzu.

## 2. Die Explorer-Seite (`GET /`)

Eine einzelne, selbst-enthaltene HTML-Seite: Vanilla-JS, Inline-CSS, per
`//go:embed` in das Binary gebacken. Kein Framework, keine CDN, keine
npm-Kette — dasselbe Muster, das `/docs` bereits etabliert (siehe
`internal/adapters/http/docs.go`): situs läuft lokal und im Feld ohne Netz,
eine Oberfläche, die ihr JavaScript aus dem Netz nachlädt, ist genau dann
leer, wenn sie gebraucht wird.

**Sie liegt unter `/`, nicht unter `/explorer`:** die Wurzel ist heute
unbelegt, und ein lokal gestarteter Dienst, dessen Adresse ohne Pfadangabe
etwas Brauchbares zeigt, erspart jede Erklärung. `gorilla/mux` matcht
`r.HandleFunc("/", …)` exakt, nicht als Präfix — die bestehenden Routen
bleiben unberührt.

### Aufbau

Acht Panels auf einer Seite, kein Client-Routing:

| Panel | Endpunkt |
|---|---|
| Index-Selbstauskunft (Kopfzeile) | `GET /v1/info` |
| Art → Habitattypen | `GET /v1/species/{conceptId}/habitat-types` |
| Art → Zeigerwerte | `GET /v1/species/{conceptId}/traits` |
| Artenliste → Habitattypen | `POST /v1/species/habitat-types` |
| **Zeigerwertanalyse** | `POST /v1/species/traits/summary` |
| Habitattyp-Detail | `GET /v1/habitat-type/{typology}/{code}` |
| Habitattyp → Arten | `GET /v1/habitat-type/{typology}/{code}/species` |
| Syntaxon → Habitattypen | `GET /v1/syntaxon/{id}/habitat-types` |

Jedes Panel zeigt **die tatsächlich abgesetzte URL** über der Antwort und
bietet die Umschaltung „formatiert ⇄ rohes JSON". Das macht die Seite zu
einem Lernwerkzeug für die API statt zu einer Hülle, die sie verbirgt.

Die Schalter `lang=de`, `area=`, `only_in_area=` liegen als globale
Steuerelemente in der Kopfzeile und gelten für alle Panels, die sie
unterstützen.

### Was die Seite NICHT tut

Keine Aggregation, kein Ranking, keine Bewertung, keine Client-Logik über
Rendern und Formatieren hinaus. Sie zeigt, was der Dienst antwortet — nicht
mehr. **Scoring/Ranking (Artenliste → gerankte Habitattypen) ist im Dienst
bewusst nicht implementiert** (siehe Foundation-Spec), und ein Frontend, das
es client-seitig nachbildete, würde eine Fähigkeit vortäuschen, die situs
nicht hat.

## 3. Namenssuche (`GET /v1/species/search`)

```
GET /v1/species/search?q=fagus&limit=20

[ {"verbatim_name": "Fagus sylvatica", "concept_id": "wcvp:concept:83891"},
  {"verbatim_name": "Fagus orientalis", "concept_id": null} ]
```

- **`q`** ist ein Teilstring, case-insensitiv gegen `verbatim_name` geprüft.
  Ein leeres oder fehlendes `q` ist `INVALID_QUERY` — nicht „alles".
- **`limit`** begrenzt die Trefferzahl, Vorgabe 20, Maximum 100. Ein
  unparsbares oder negatives `limit` ist `INVALID_QUERY`.
- Sortiert nach `verbatim_name`, damit dieselbe Anfrage stabil dieselbe
  Reihenfolge liefert.
- **`concept_id: null` wird ausgeliefert, nicht ausgefiltert.** Ein Name, den
  der Index führt, aber nicht auflösen konnte, ist Teil der Wahrheit über
  diesen Index — ihn zu verstecken, täuschte eine höhere Auflösungsrate vor
  als die gemessenen 83,9 %. Das Frontend zeigt solche Treffer als nicht
  abfragbar an.
- SQL bleibt ein statisches Statement mit `?`-Platzhaltern; das Suchmuster
  wird als Parameter gebunden, nie in die Anweisung interpoliert (gosec
  G201/G202).

## 4. Zeigerwertanalyse (`POST /v1/species/traits/summary`)

```
POST /v1/species/traits/summary
{ "concept_ids": ["wcvp:concept:83891", "wcvp:concept:2692970"] }
```

```json
{
  "requested": 18,
  "known": 16,
  "unknown": [ {"concept_id": "wcvp:concept:99", "reason": "unknown_concept"} ],
  "vocabularies": {
    "eive": {
      "vocab_version": "1.0",
      "dimensions": {
        "M": { "mean": 4.644, "mean_niche_weighted": 4.657, "sd": 1.31,
               "min": 2.1, "max": 7.8, "n": 14, "n_missing": 2 }
      }
    }
  }
}
```

### Gemessene Datenlage (nicht angenommen)

Gegen `out/situs-0.3.0.sqlite`:

| Vokabular | Dimensionen | Wertebereich | Nischenbreite |
|---|---|---|---|
| `eive` (1.0) | L, M, N, R, T | 0–10 | durchgängig vorhanden, 0,0798–10,0 (Mittel 3,49) |
| `tichy2023` (2.0) | L, M, N, R, S, T | 1–9 bzw. 1–12 | keine |
| `midolo2023` (3) | 5 Störungsindikatoren | 0–0,96 bzw. 0–2,63 | keine |

### Rechenregeln

- **Strikt pro Vokabular und Dimension.** Nie über Vokabulare mitteln, nie
  zwischen Skalen umrechnen: EIVE 0–10 und Tichý 1–12 sind verschiedene
  Skalen, ihr Mittelwert wäre eine erfundene Zahl.
- **`mean`** ist das arithmetische Mittel über alle Arten mit Wert in dieser
  Dimension.
- **`mean_niche_weighted`** gewichtet mit `1 / niche_width` — eine schmale
  Nische ist ein präziserer Standortanzeiger als ein Generalist. Gemessen an
  einer realen 40-Arten-Buchenwaldliste (EIVE/M) liegen ungewichtetes und
  gewichtetes Mittel bei 4,644 gegenüber 4,657, das größte Einzelgewicht bei
  4,3 % — die Formelwahl ist damit eine dokumentierte Konvention, keine
  ergebnisprägende Weiche.
- **Arten mit `niche_width <= 0` fließen NICHT in das gewichtete Mittel ein**
  und werden gezählt. In den aktuellen Daten kommt das 0-mal vor; ein
  Re-Ingest darf den Dienst trotzdem nicht in eine Division durch Null
  laufen lassen, und ein erfundener Ersatzwert wäre die unehrlichere Rettung.
- **`mean_niche_weighted` FEHLT** (Feld nicht gesetzt) bei Vokabularen ohne
  Nischenbreiten. Es wird nie mit dem ungewichteten Wert gefüllt — ein Client
  darf beide nicht verwechseln können.
- **`sd` fehlt bei `n < 2`.** Nicht `0`: eine Streuung von null ist eine
  Aussage, „aus einem Wert lässt sich keine Streuung berechnen" eine andere.
- **`n` und `n_missing` stehen pro Dimension**, nicht global: EIVE deckt
  L/M/N/R/T unterschiedlich gut ab, und ein Mittel aus 3 von 18 Arten ist
  etwas anderes als eines aus 17 von 18.
- **Unbekannte Concept-IDs** werden wie in der Batch-Route ausgewiesen
  (`unknown_backbone` bei falschem Präfix, `unknown_concept` sonst), nie
  stillschweigend übergangen. Eine Anfrage, von der nichts auflösbar war, ist
  ein normales 200 mit leeren Vokabularen — kein Fehler.
- Eine leere `concept_ids`-Liste ist `INVALID_QUERY`.

### Wo das lebt

Die Statistik gehört in `internal/application` (die erste Route, die im
Dienst tatsächlich *rechnet* statt nur zu lesen), der Datenzugriff in den
sqlite-Adapter. Die Berechnung selbst bekommt eigene Tests für ihre Kanten:
`n = 0`, `n = 1`, fehlende Nischenbreiten, `niche_width <= 0`, gemischte
Vokabulare.

## 5. Fehlerbehandlung

| Fall | Verhalten |
|---|---|
| `q` fehlt/leer | `400 INVALID_QUERY` |
| `limit` unparsbar, `<= 0` oder `> 100` | `400 INVALID_QUERY` |
| Suche ohne Treffer | `200` mit leerer Liste (kein 404) |
| `concept_ids` leer/fehlend | `400 INVALID_QUERY` |
| Alle Concept-IDs unbekannt | `200`, `unknown` gefüllt, `vocabularies` leer |
| Dimension ohne einen einzigen Wert | Dimension fehlt in der Antwort (keine Null-Zeile) |

Es bleibt bei den drei bestehenden Fehlercodes (`INVALID_QUERY`,
`NOT_FOUND`, `INTERNAL_ERROR`); es kommt keiner hinzu.

## Prüfbare Zusagen

- `GET /` liefert HTML aus, das ohne Netzzugang vollständig funktioniert:
  kein `<script src="http…">`, kein `<link href="http…">`.
- Die drei neuen Routen stehen in `openapi.yaml` und in `api/openapi/openapi.yaml`
  (byte-identisch), `TestRoutesMatchOpenAPISpec` und
  `TestOpenAPICopiesAreIdentical` bleiben grün.
- `mean_niche_weighted` erscheint **ausschließlich** bei `eive` — ein Test
  hält fest, dass `tichy2023` und `midolo2023` das Feld nicht tragen.
- Eine Artenliste, deren Arten in einer Dimension alle keine Nischenbreite
  haben, liefert dort `mean`, aber kein `mean_niche_weighted`.
- `sd` fehlt bei `n < 2`, statt `0` zu sein.
- Die Suche liefert Treffer mit `concept_id: null` mit aus, statt sie zu
  filtern.
- Es kommt keine neue Go-Abhängigkeit hinzu (`gomodguard_v2` hält das).
- Der Serve-Pfad bleibt autark: keine der neuen Routen ruft hostus oder
  irgendeinen Upstream (`internal/app/arch_test.go` bleibt grün).

## Offene Punkte (bewusst nicht gelöst)

- **Das JavaScript der Explorer-Seite bleibt ungetestet.** Getestet werden
  die Go-Seite (Route liefert HTML, Suche und Analyse liefern korrekte
  Ergebnisse) und der Contract. Ein Test-Framework für Browser-JS in ein
  Projekt zu holen, dessen Abhängigkeitsliste bewusst kurz ist, wäre für ein
  Testwerkzeug unverhältnismäßig.
- **Abundanz-/Deckungsgewichtung** der Zeigerwertanalyse: bewusst nicht
  enthalten. Der erklärte Use-Case (Exkursions-App notiert Arten) liefert
  keine Deckungsgrade. Später additiv ergänzbar, ohne bestehende Clients zu
  brechen.
- **Scoring/Ranking** bleibt out of scope, im Dienst wie im Frontend.
- **Keine Autorisierung, kein Rate-Limiting** für die neuen Routen — situs
  ist ein lokaler, schreibgeschützter Dienst, daran ändert eine Oberfläche
  nichts.
