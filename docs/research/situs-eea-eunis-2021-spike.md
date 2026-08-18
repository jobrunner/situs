# situs — Spike: Was gibt die EUNIS-2021-Veröffentlichung der EEA maschinenlesbar her?

Stand: 2026-08-18. Sondierung (kein Implementierungs-Task), Vorlauf zum
geplanten Dienst **situs** (Habitat-/Vegetationswissen; Hostus bleibt reiner
Namens-Crosswalk `verbatim → Konzept`). Methodik wie beim
[ESy-Spike](sp9-esy-spike.md): öffentliche EEA-/EUNIS-Quellen sichten, belegen
was maschinenlesbar vorliegt, offene Punkte klar als „am Rohartefakt zu prüfen"
markieren.

## Die Frage

situs soll (a) je **Pflanze** sagen, in welchen Habitaten sie **Kennart /
konstante Art / dominante Art** ist, (b) je **Habitat** die Artenlisten dieser
drei Rollen liefern, (c) die **Pflanzengesellschaften** (Syntaxa: Klasse,
Ordnung, Verband, idealerweise Assoziation) eines EUNIS-Habitats liefern, und
(d) den **EUNIS 2012 ↔ 2021-Crosswalk mit Coverage** nutzen. Vor einer
Fundament-Spec ist zu klären, was davon die EEA tatsächlich **maschinenlesbar**
hergibt — statt auf Annahmen zu spezifizieren.

## Befund A: Klassifikation + Crosswalks — ein maschinenlesbares XLSX

Die **EUNIS terrestrial habitat classification 2021_1 including crosswalks**
liegt als **einzelnes Excel (~1,4 MB)** vor, plus zwei abgeleitete Varianten
(„… with crosswalks to Annex I in separate rows.xlsx" ~59 KB; „… to Red List in
separate rows.xlsx" ~50 KB). Bezug über die EEA
([Datensatzseite][eea-2021], [EUNIS-Referenz 2487][ref2487]). Ein Excel, kein
Crawl — Beschaffung trivial.

Die Klassifikation trägt laut EEA **Crosswalks auf Level 3** zu: Habitats
Directive Annex I, European Red List of Habitats, Bern Convention Res. 4,
MAES/IUCN-Ökosystemen, Corine Land Cover — **und zur Euroveg Checklist 2016
Syntaxa**. Forst- und Heide-Gruppen tragen zusätzlich Crosswalks zu einer
Revision von 2017.

## Befund B: Die drei Rollen (Kennart/konstant/dominant) sind bereits gerechnet

Das ist der wichtigste Fund für die Machbarkeit: **diagnostische, konstante und
dominante Arten je Habitat existieren als fertig berechneter, publizierter
Datensatz** — die „characteristic species combination" aus Chytrý et al. 2020
(*Applied Vegetation Science*, EUNIS-ESy). Sie werden aus den Vegetations-
aufnahmen der European Vegetation Archive (EVA) über **Fidelität** (→
diagnostisch) und **Stetigkeit/Deckung** (→ konstant/dominant) bestimmt und
sind Teil des ESy-Zenodo-Records (`Characteristic-species-combinations.xlsx`,
~0,4 MB, **CC BY 4.0**; siehe [ESy-Spike](sp9-esy-spike.md)) sowie in jedem
EUNIS-Habitat-Factsheet ausgewiesen.

**Konsequenz:** die im Brainstorming befürchtete Lücke „Konstanz/Dominanz
bräuchten eine eigene Aufnahmen-DB" entfällt. Beide sind bereits gerechnet und
redistribuierbar. situs braucht **keine** eigene Vegetationsdatenbank für die
drei Rollen — es konsumiert die fertige Kombinationstabelle.

## Befund C: Habitat ↔ Syntaxon (Euroveg Checklist 2016) — vorhanden, Level 3

Der Habitat→Pflanzengesellschaft-Bezug ist **kein Nebenprodukt, sondern eigens
entwickelt und dokumentiert**: „Development of vegetation syntaxa crosswalks to
EUNIS habitat classification" (Schaminée, Chytrý et al.; EEA-Report, u. a.
[Report 2014][syntaxa-report]). Die Zuordnung steckt als Spalte in der
2021-XLSX (Euroveg Checklist 2016 = Mucina et al.-Syntaxonomie: Klasse →
Ordnung → **Verband**).

Zwei Einschränkungen, ehrlich:

- **Granularität = Level 3.** Der Syntaxa-Crosswalk ist wie die anderen
  Crosswalks auf **EUNIS-Level 3** geschlüsselt. „Verband je Habitat" ist damit
  bis Level 3 beantwortbar; **Assoziation** (deine Wunsch-Feinstufe) und
  Level-4-Habitate sind so **nicht garantiert** abgedeckt — das ist genau die
  „Level müsste man herausarbeiten"-Frage, und die Antwort aus der Datenlage
  lautet: robust bis Level 3, darunter unsicher.
- **many-to-many bestätigt.** Ein Habitat referenziert mehrere Syntaxa und ein
  Syntaxon mehrere Habitate — dein „dieselbe Gesellschaft in mehreren
  Habitaten" ist im Crosswalk strukturell angelegt, nicht nachzurüsten.

## Befund D: EUNIS 2012 ↔ 2021-Crosswalk + Coverage-Qualifier

Der Versions-Crosswalk existiert. Bestätigt durch das R-Paket `eunis.habitats`,
das exakt ein Paar `eunis_2012_code` ↔ `eunis_t_2021_code` führt
([Referenz][rpkg-crosswalk]) — d. h. die Zuordnung ist maschinenlesbar in der
EEA-Rohdatei enthalten (das Paket normalisiert allerdings einen etwaigen
Qualifier weg).

Der von dir genannte **Coverage/Abdeckungs-Qualifier** ist für EUNIS-Crosswalks
belegt: die Symbolik **`=` (vollständige Entsprechung), `<`, `>`, `#`**
(schmaler/breiter/partiell) wird in der EUNIS-Crosswalk-Dokumentation verwendet
(explizit im marinen Crosswalk beschrieben, dieselbe Systematik in den
terrestrischen Tabellen). **Wo 2012 ≡ 2021 (`=`)** greift dein Argument: dann
sind 2012-gebundene Zusatzinformationen (der 2012er Bestimmungsschlüssel,
nicht-pflanzliche Kriterien) verlustfrei übertragbar.

→ **Am Rohartefakt zu verifizieren** (Build-SP, nicht hier): die exakte
Spaltenbezeichnung und die tatsächlich benutzten Qualifier-Werte der
2012↔2021-Spalte in der 2021-XLSX. Dass Crosswalk **und** Qualifier-Systematik
existieren, ist belegt; ihre genaue Kodierung liest der Ingest verbatim.

## Befund E: Lizenz

- **Charakterart-Kombinationen + ESy:** **CC BY 4.0** (Zenodo, redistribuierbar
  — siehe ESy-Spike).
- **EUNIS-2021-XLSX (EEA):** unter **EEA-Datenpolitik** (freie Weiterverwendung
  mit Quellenangabe; EEA-Daten sind grundsätzlich frei reproduzierbar). Der
  exakte Lizenztext ist beim Pinnen zu zitieren — kein Blocker erkennbar, aber
  nicht wörtlich als CC-Kennung ausgewiesen wie bei Zenodo.

## Die Scope-Grenze (unverändert gegenüber dem ESy-Spike)

Eine **volle Klassifikation Aufnahme → EUNIS-Habitat** bleibt außerhalb: die
ESy-Regeln brauchen **Deckungswerte (alle 304 Regeln)** und teils
**Region/Koordinate (168 Regeln)** — Aufnahme-Metadaten. situs kann deshalb:

- **ohne** Deckung/Region: über die **Fakten-Schicht** (Befund B/C) aus einer
  Artenliste die *möglichen* Habitate + ihre Gesellschaften eingrenzen
  („Handvoll statt eine") — genau der realistische Weg aus dem Brainstorming.
- **mit** Deckung/Region (falls die App sie liefert): über **ESy-Regeln** und
  den **2012er Schlüssel** als *Motoren* nachschärfen.

Beides ist datenseitig gedeckt; die volle Plot-Klassifikation ist bewusst
nicht das Ziel.

## Verdikt

| Baustein | Datenlage | Status |
|---|---|---|
| Arten-Rollen (Kennart/konstant/dominant) je Habitat | ESy „characteristic species combinations", CC BY 4.0, fertig gerechnet | **grün** |
| Habitat ↔ Syntaxon (Klasse/Ordnung/Verband) | Euroveg-2016-Crosswalk in 2021-XLSX | **grün bis Level 3**, Assoziation/Level 4 unsicher |
| EUNIS 2012 ↔ 2021 + Coverage-Qualifier | in 2021-XLSX; Qualifier-Systematik belegt | **grün**, exakte Spalte/Werte am Rohartefakt lesen |
| Beschaffung/Lizenz | 2–3 XLSX + Zenodo, CC BY / EEA-Politik | **grün** |
| Namens-Auflösung ESy/EUNIS-Namen → Konzept | Hostus-Crosswalk (~57 % Boden, siehe ESy-Spike) | **messbar im Build-SP** |

**Kein Blocker.** Alles, was situs' Fundament braucht, ist maschinenlesbar,
beschaffbar und (im Kern) redistribuierbar. Die einzige echte inhaltliche
Grenze ist die **Level-3-Granularität** der Crosswalks — die betrifft die
Feinstufe (Assoziation), nicht das Grundvorhaben.

## Nächster Schritt

Fundament-Spec für situs auf dieser Datenlage: Datenmodell (Habitat ×
Version × Level; Artenrolle; Syntaxon; Habitat↔Syntaxon m:n;
Versions-Crosswalk mit Qualifier), Ingest der 2–3 XLSX + der ESy-
Kombinationstabelle, Namens-Crosswalk der Artnamen über Hostus, Read-Queries
für beide Richtungen. Die zwei am Rohartefakt zu klärenden Details
(Syntaxa-Tiefe, Qualifier-Spalte) gehören als erste, messende Ingest-Schritte
hinein — nicht als Annahme in die Spec.

## Quellen

- EUNIS terr. 2021 (EEA): [Datensatz][eea-2021] · [Referenz 2487][ref2487]
- Charakterart-Kombinationen + ESy: [ESy-Spike](sp9-esy-spike.md),
  Zenodo DOI 10.5281/zenodo.3841729 (CC BY 4.0)
- Syntaxa-Crosswalk-Report: [Schaminée/Chytrý 2014][syntaxa-report]
- Versions-Crosswalk-Spalten: R-Paket [`eunis.habitats`][rpkg-crosswalk]

[eea-2021]: https://www.eea.europa.eu/data-and-maps/data/eunis-habitat-classification-1/eunis-terrestrial-habitat-classification-review-2021/eunis-terrestrial-habitat-classification-2021
[ref2487]: https://eunis.eea.europa.eu/references/2487
[syntaxa-report]: https://sdi.eea.europa.eu/webdav/datastore/public/eea_t_eunis-documentation_p_1940-ongoing_v01_r00/EUNIS%20habitat%20classification%20revision%20documentation/Report%202012%20Vegetation%20syntaxa%20crosswalks%20to%20EUNIS%20habitat%20classification%20update%2030-05-2014.pdf
[rpkg-crosswalk]: https://rmagno.eu/eunis.habitats/reference/crosswalk.html
