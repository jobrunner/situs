# situs — Fundament (Fakten-Schicht bis Verband): Design-Spec

Stand: 2026-08-18. Ergebnis des Brainstormings. Datenlage belegt im
[EEA-EUNIS-2021-Spike](../../research/situs-eea-eunis-2021-spike.md) und im
[ESy-Spike](../../research/sp9-esy-spike.md). Diese Spec beschreibt **nur das
Fundament** (die statische Fakten-Schicht); Scoring und Regel-Motoren sind
bewusst spätere, eigene Schnitte.

> Hinweis: situs wird ein **eigenes Repository** (Beschluss im Brainstorming).
> Diese Spec entsteht vorerst im Hostus-Repo neben den Spikes und wandert beim
> Repo-Aufsetzen mit.

## Ziel

Ein lokaler, read-only Dienst **situs**, der Habitat- und Vegetationsfakten
liefert, so dass eine Exkursions-App aus einer Artenliste die *möglichen*
EUNIS-Habitate (samt Pflanzengesellschaften) eingrenzen kann — **ohne**
Deckungs-/Standortdaten. situs beantwortet Einzel-Fakten in beide Richtungen:
Art → Habitate/Rollen und Habitat → Artenlisten/Syntaxa.

Zwei Anforderungen aus der **Naturschutzarbeit** sind von Anfang an Teil des
Fundaments, nicht Nachrüstung:

- **FFH-Bezug:** ein Crosswalk EUNIS-Habitattyp ⇄ **Anhang-I-Habitattyp**
  (im Deutschen: FFH-Lebensraumtyp, LRT). Nicht immer vorhanden — aber wo
  vorhanden, hochrelevant, weil er Vegetationsbefunde an das Schutzrecht
  anbindet.
- **Deutsch:** Habitattyp-**Namen**, **Beschreibungen** und
  **Bestimmungsschlüssel** zusätzlich auf Deutsch, um die Einstiegshürde für
  Anwender drastisch zu senken.

## Nicht-Ziele (Abgrenzung)

- **Kein Scoring/Ranking** „X Arten → gerankte Habitate" (späterer Schnitt S2).
- **Keine ESy-/Schlüssel-Motoren** — die brauchen Deckung/Region (S3/S4).
- **Keine volle Plot-Klassifikation** (Aufnahme → Habitat) — systembedingt
  außerhalb eines Fakten-Dienstes (siehe ESy-Spike).
- **Keine Assoziations-Ebene** — die freie Datenlage reicht bis **Verband**;
  Assoziation wird über eine EVA-Anfrage erst verfolgt, wenn ein belastbares
  wissenschaftliches Projekt sie trägt.
- **situs hält keine Namenslogik** — Namensauflösung ist und bleibt Hostus.

## Architektur

situs ist ein **Hostus-Zwilling**: hexagonal (domain/application/ports/
adapters/app), lokaler **SQLite**-Index, Ingest aus **gepinnten** Artefakten,
dieselben Gates (`verify`, Mutation, Lint, Arch/Debt-Guard). Begründung: der
Zuschnitt (read-only, versionierte gepinnte Quellen, lokaler Index) ist mit
Hostus identisch; Muster, Toolchain und Betriebserfahrung werden 1:1
wiederverwendet.

**Grenze zu Hostus.** Beim **Ingest** ruft situs Hostus (`POST /v1/match`),
um jeden Artnamen der Quellen auf eine stabile **Konzept-ID** zu normalisieren;
situs speichert die Konzept-ID. Zur **Laufzeit** ist situs damit autark für
Abfragen per Konzept-ID. Für Abfragen per **verbatim Name** delegiert situs
diese eine Auflösung an Hostus (oder die App löst vorab via Hostus auf und
fragt situs per Konzept-ID) — situs selbst implementiert keine Namenslogik.

```
Exkursions-App ──(name|conceptId)──▶ situs ──(conceptId)──▶ lokaler Index
                                       │
                                       └─(name→conceptId, nur wenn per Name)──▶ Hostus
Ingest:  EEA-XLSX + ESy-XLSX + Euroveg 2016 ──▶ situs-Ingest ──(alle Namen)──▶ Hostus ──▶ conceptIds
```

## Datenmodell

### Ubiquitous Language

Zwei Begriffe tragen das Modell und werden konsequent so benannt:

- **Habitattypen-System** (`habitat_typology`) — ein *Klassifikationssystem*
  abstrakter Habitattypen in einer bestimmten Fassung: EUNIS 2021, EUNIS 2012,
  FFH-Anhang I. Ein weiteres System (nationale Biotoptypen, Palaearctic, …)
  ist später **eine Zeile**, kein Schema-Umbau.
- **Habitattyp** (`habitat_type`) — ein *abstrakter Typ* innerhalb eines
  Systems, ausdrücklich **kein** konkretes Biotop/Habitat in der Landschaft.
  Ein Typ ist nie „für sich" identifiziert, sondern immer über
  `(typology, code)` — z. B. `(eunis@2021, 'R22')` vs. `(annex1, '6210')`.

Daraus folgt eine Vereinfachung: **EUNIS-Versions-Crosswalk und
EUNIS↔Anhang-I-Crosswalk sind derselbe Begriff** — eine Entsprechung zwischen
zwei Habitattypen mit Coverage-Qualifier. Beide Quellen benutzen dieselben
Qualifier (`=`/`<`/`>`/`#`); das ist kein Zufall, sondern dasselbe Konzept.
Es gibt deshalb **eine** Crosswalk-Tabelle, nicht zwei.

Zur Benennung von Anhang I: international ist **„Annex I habitat type"** der
gebräuchliche Begriff (EEA-Daten, Richtlinie 92/43/EWG, EU-Fachliteratur);
nationale Bezeichnungen wie **„FFH-LRT"** unterscheiden sich je Land. Der
Code/Bezeichner bleibt deshalb `annex1`, und **„FFH-Lebensraumtyp" ist ein
deutsches Label** — es lebt in `localization`, genau wie die Habitatnamen.

```sql
-- Habitattypen-System (Klassifikation + Fassung)
habitat_typology(
  id TEXT PRIMARY KEY,                  -- 'eunis@2021' | 'eunis@2012' | 'annex1'
  scheme TEXT, version TEXT,            -- ('eunis','2021') | ('annex1','92/43/EEC')
  name TEXT, source_ref TEXT            -- gepinnte Quelle (URL/DOI)
)

-- Abstrakter Habitattyp innerhalb eines Systems
habitat_type(
  typology_id TEXT,                     -- -> habitat_typology.id
  code TEXT,                            -- 'R22' | '6210'
  level INTEGER,                        -- Hierarchietiefe, sofern das System eine hat
  name_en TEXT,
  parent_code TEXT,                     -- Hierarchie innerhalb desselben Systems
  priority INTEGER,                     -- nur Anhang I: prioritärer LRT (sonst NULL)
  PRIMARY KEY (typology_id, code)
)

-- Entsprechung zwischen zwei Habitattypen — EINE Mechanik für
-- Versions-Crosswalk (eunis@2012 -> eunis@2021) UND System-Crosswalk
-- (eunis@2021 -> annex1). Weitere Systeme brauchen keine neue Tabelle.
habitat_type_crosswalk(
  from_typology TEXT, from_code TEXT,
  to_typology   TEXT, to_code   TEXT,
  qualifier TEXT,                       -- '='|'<'|'>'|'#' (verbatim aus Quelle)
  PRIMARY KEY (from_typology, from_code, to_typology, to_code)
)

-- Syntaxon (Euroveg Checklist 2016), Hierarchie bis Verband
syntaxon(
  id TEXT PRIMARY KEY, rank TEXT,       -- 'class'|'order'|'alliance'
  name TEXT, parent_id TEXT
)

-- Habitattyp ⇄ Syntaxon, many-to-many (Level 3)
habitat_type_syntaxon(
  typology_id TEXT, code TEXT, syntaxon_id TEXT,
  PRIMARY KEY (typology_id, code, syntaxon_id)
)

-- Lokalisierung: Namen, Beschreibungen, Bestimmungsschlüssel je Sprache.
-- Generisch über Entitätstypen, damit Habitattyp und Syntaxon dieselbe
-- Mechanik nutzen und weitere Sprachen später nur Daten sind, kein Schema.
localization(
  entity_type TEXT,                     -- 'habitat_type'|'syntaxon'
  entity_key  TEXT,                     -- 'eunis@2021:R22' | 'annex1:6210' | syntaxon_id
  lang        TEXT,                     -- 'de' (BCP 47)
  field       TEXT,                     -- 'name'|'description'|'key'
  value       TEXT,
  source      TEXT,                     -- Herkunft, z.B. 'ffh-richtlinie-de'|'bfn'
  provenance  TEXT,                     -- 'official'|'curated'|'derived'
  derived_from TEXT,                    -- bei 'derived': z.B. 'annex1:6210 qualifier=='
  PRIMARY KEY (entity_type, entity_key, lang, field, source)
)

-- Art-Rolle je Habitattyp (aus ESy characteristic species combinations)
species_role(
  typology_id TEXT, code TEXT,          -- der Habitattyp (i.d.R. eunis@<version>)
  concept_id TEXT,                      -- aus Hostus; NULL wenn unauflösbar
  verbatim_name TEXT,                   -- Quell-Name, IMMER gesetzt (kein stiller Verlust)
  role TEXT,                            -- 'diagnostic'|'constant'|'dominant'
  fidelity REAL, constancy REAL,        -- optional, falls die Quelle sie führt
  PRIMARY KEY (typology_id, code, verbatim_name, role)
)
```

Anmerkungen:
- `concept_id` ist die Klammer zu Hostus. Unauflösbare Namen werden **nicht
  verworfen** — `verbatim_name` bleibt erhalten, `concept_id` ist NULL. Die
  Auflösungsquote wird beim Ingest **gemessen und geloggt** (kein stiller
  Deckel; siehe ESy-Spike: Boden ≈ 57 %).
- `level` und die Syntaxa-Tiefe sind **am Rohartefakt zu messen** (erster
  Ingest-Schritt), nicht anzunehmen. `level`/`parent_code` sind optional —
  ein System ohne Hierarchie lässt sie NULL.
- **Anhang I ist ein eigenes Habitattypen-System, kein EUNIS-Attribut.** Die
  Entsprechung ist m:n und *nicht immer vorhanden* — fehlende Zuordnung ist der
  Normalfall und wird als Abwesenheit von Crosswalk-Zeilen abgebildet, nie als
  Platzhalter-Code.
- **`priority` ist bewusst eine nullable Spalte**, kein generisches
  Attribut-System: es ist heute das einzige systemspezifische Merkmal. Kämen
  mehrere hinzu, wäre das der Moment für eine eigene Attributtabelle — jetzt
  wäre sie YAGNI.
- **Lokalisierung ist Overlay, nie Ersatz.** `habitat_type.name_en` bleibt
  immer die Identität; `de` legt sich darüber. `provenance` trennt drei
  Qualitäten: `official` (amtlicher deutscher Wortlaut der FFH-Richtlinie),
  `curated` (händisch gepflegt), `derived` (siehe unten). Ein Client kann so
  entscheiden, ob er Abgeleitetes anzeigt.
- **Deutsche Einstiegsnamen für EUNIS über den Anhang-I-Crosswalk.** EUNIS hat
  keine amtliche deutsche Übersetzung, die Anhang-I-Typen haben eine. Wo ein
  EUNIS-Typ über `habitat_type_crosswalk` mit Qualifier `=` (vollständige
  Entsprechung) auf einen Anhang-I-Typ zeigt, wird dessen amtlicher deutscher
  Name als `provenance='derived'`, `derived_from='annex1:<code> qualifier=='`
  als **Einstiegshilfe** erzeugt — klar markiert, niemals als offizieller
  EUNIS-Name ausgegeben. Nur `=` wird abgeleitet; `<`/`>`/`#` sind dafür zu
  ungenau.

## Ingest (alle Quellen gepinnt, per Manifest)

1. **EUNIS 2021 XLSX** (EEA) → `habitat_typology` (`eunis@2021`, `eunis@2012`),
   `habitat_type`, `habitat_type_crosswalk` (Versions-Crosswalk 2012↔2021 **und**
   EUNIS→`annex1`, beides mit Qualifier), `habitat_type_syntaxon`, `syntaxon`
   (via Euroveg-2016-Spalte). Für den Anhang-I-Teil ist die Variante
   „… with crosswalks to Annex I in separate rows.xlsx" die bevorzugte Eingabe,
   weil sie bereits je Zeile normalisiert ist.
2. **ESy `Characteristic-species-combinations.xlsx`** (Zenodo, CC BY 4.0) →
   `species_role` (die drei Rollen; Fidelität/Stetigkeit falls vorhanden).
3. **Euroveg Checklist 2016** → `syntaxon`-Hierarchie (Klasse/Ordnung/Verband),
   soweit nicht schon aus (1) ableitbar.
4. **FFH-Richtlinie Anhang I, deutsche Fassung** (EUR-Lex; EU-Recht ist in
   allen Amtssprachen amtlich veröffentlicht) → `habitat_typology('annex1')` +
   `habitat_type` (Codes, `priority`) + `localization(lang='de', field='name',
   provenance='official')`. Optional ergänzend **BfN-Steckbriefe** der in
   Deutschland vorkommenden LRTs → `field='description'`/`'key'`,
   `source='bfn'` (Lizenz beim Pinnen prüfen).
5. **Namens-Crosswalk:** alle `verbatim_name` gebündelt an Hostus `POST
   /v1/match` → `concept_id` eintragen; Trefferquote reporten.
6. **Abgeleitete de-Labels** (letzter Schritt, rein rechnerisch): je
   Crosswalk-Zeile EUNIS→`annex1` mit Qualifier `=` den amtlichen deutschen
   Namen des Anhang-I-Typs als `provenance='derived'` für den EUNIS-Typ
   erzeugen.

**Zwei messende Vorab-Schritte** (Ergebnis fließt in die Ingest-Logik, nicht in
Annahmen): (a) tatsächliche **Syntaxa-Tiefe** der 2021-XLSX (endet sie bei
Verband? gibt es Level-4-Bezüge?); (b) exakte **Spaltenbezeichnung und
Wertemenge** des Versions-Qualifiers.

## Read-API (v1, im Hostus-Stil)

Habitattypen adressieren sich einheitlich über `{typology}/{code}` — dieselbe
Route trägt EUNIS und Anhang I, und ein künftiges System braucht keine neuen
Endpunkte. `typology` ist `eunis@2021` (Default, wenn weggelassen), `eunis@2012`
oder `annex1`.

- `GET  /v1/species/{conceptId}/habitat-types` → `[{typology, code, level,
  name_en, name_de?, role, syntaxa:[{id,rank,name}]}]`
- `POST /v1/species/habitat-types` → Batch per verbatim Name (situs → Hostus →
  Konzept → Fakten); liefert je Eingabe die Auflösung + Typen/Rollen.
- `GET  /v1/habitat-type/{typology}/{code}` → Typ + `species:{diagnostic:[],
  constant:[], dominant:[]}` + `syntaxa:[]` + `crosswalks:[{typology, code,
  qualifier}]`. Die Crosswalk-Liste trägt **beides**: andere EUNIS-Fassungen
  und Anhang-I-Entsprechungen — leer, wenn es keine gibt (der Normalfall bei
  Anhang I ist ausdrücklich erlaubt).
- `GET  /v1/habitat-type/{typology}/{code}/species?role=` → gefilterte
  Artenliste.
- `GET  /v1/habitat-type/annex1/{code}` → derselbe Endpunkt liefert die
  Naturschutz-Einstiegsrichtung („ich habe einen LRT — welche EUNIS-Typen und
  welche Arten gehören dazu?"), weil die Crosswalks beidseitig abfragbar sind.
- `GET  /v1/syntaxon/{id}/habitat-types` → Typen zu einer Gesellschaft (zeigt
  das m:n: dieselbe Gesellschaft in mehreren Habitattypen).

**Sprache.** Jeder Endpunkt akzeptiert `?lang=de` (bzw. `Accept-Language`);
Default ist `en`. Lokalisierte Felder werden **additiv** ausgeliefert
(`name` bleibt der englische Originalname, `name_de` kommt hinzu) — nie
ersetzend, damit IDs/Namen stabil bleiben. Jedes abgeleitete Label trägt seine
Herkunft mit (`name_de_provenance: "derived"`), so dass eine UI Abgeleitetes
kennzeichnen oder ausblenden kann.
- Konvenienz (ableitbar): **Ko-Kennarten** — Art → ihre Habitate → deren Arten
  je Rolle (deine „eine Kennart → weitere Kennarten"-Idee, ohne neuen Speicher).
- Betrieb wie Hostus: `GET /openapi`, `GET /metrics`, `GET /health/{live,ready}`.

## Fehlerbehandlung

Fehlerformat identisch zu Hostus (`{"error":{"code","message"}}`). Codes:
`INVALID_QUERY` (auch: unbekannte `typology`), `NOT_FOUND` (unbekannter
Habitattyp / unbekanntes Konzept),
`UNRESOLVABLE` (verbatim Name nicht auf ein Konzept auflösbar — von Hostus
durchgereicht), `UPSTREAM_UNAVAILABLE` (Hostus beim Namenspfad nicht
erreichbar), `INTERNAL_ERROR`.

## Test-/Qualitätsansatz

TDD wie in Hostus. Ingest gegen kleine, echte XLSX-Ausschnitte als Fixtures.
Read-Adapter gegen einen In-Memory-SQLite mit gesäten Fakten. Dieselben Gates
(`verify`, Mutation `Not covered=0`, Lint, Arch/Debt). Ein Ingest-Report als
Artefakt (Auflösungsquote, Syntaxa-Tiefe, Qualifier-Werte) — messbar, nicht
angenommen.

## Offene, im Build zu klärende Punkte (nicht Annahme, sondern erste Tasks)

1. Syntaxa-Tiefe der 2021-XLSX (Verband-Ende bestätigen).
2. Exakte Qualifier-Spalte/Werte des 2012↔2021-Crosswalks.
3. Namens-Auflösungsquote ESy/EUNIS → Hostus-Konzept (Boden ≈ 57 % messen).
4. EEA-Lizenztext beim Pinnen wörtlich zitieren (kein Blocker erwartet).
5. **Annex-I-Abdeckung messen:** wie viele EUNIS-Habitate haben überhaupt einen
   Anhang-I-Bezug, und wie verteilen sich die Qualifier (`=` vs. `<`/`>`/`#`)? Die
   `=`-Quote bestimmt direkt, wie viele deutsche Einstiegsnamen ableitbar sind.
6. **Deutsche Quellen fixieren:** EUR-Lex-Fassung der FFH-Richtlinie (Fassung/
   Änderungsstand pinnen, inkl. Beitritts-Ergänzungen); BfN-Steckbriefe nur
   aufnehmen, wenn die Nutzungsbedingungen das hergeben — sonst bleibt es bei
   den amtlichen Namen aus der Richtlinie.
