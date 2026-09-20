-- No table here declares a FOREIGN KEY, deliberately. The sources are pinned
-- third-party artifacts: a crosswalk whose target code is absent from the
-- classification sheet would abort the whole ingest over one upstream
-- inconsistency, which is the wrong trade for a rebuildable local index.
--
-- Referential integrity rests on two things instead: the ingest is the single
-- writer and runs each stage in one transaction, and the read path detects a
-- dangling reference and reports it as an index inconsistency rather than
-- serving garbage. That detection is the mitigation, not an afterthought.
--
-- db.go still sets PRAGMA foreign_keys=ON. That pragma is currently INERT —
-- with no constraints declared there is nothing for it to enforce, and it must
-- not be read as evidence that references are checked. It is set so the
-- behaviour is correct from the start should constraints ever be added.

CREATE TABLE IF NOT EXISTS habitat_typology (
  id         TEXT PRIMARY KEY,
  scheme     TEXT NOT NULL,
  version    TEXT NOT NULL DEFAULT '',
  name       TEXT NOT NULL DEFAULT '',
  source_ref TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS habitat_type (
  typology_id TEXT NOT NULL,
  code        TEXT NOT NULL,
  level       INTEGER,
  name_en     TEXT NOT NULL DEFAULT '',
  parent_code TEXT NOT NULL DEFAULT '',
  priority    INTEGER,
  PRIMARY KEY (typology_id, code)
);

CREATE TABLE IF NOT EXISTS habitat_type_crosswalk (
  from_typology TEXT NOT NULL,
  from_code     TEXT NOT NULL,
  to_typology   TEXT NOT NULL,
  to_code       TEXT NOT NULL,
  qualifier     TEXT NOT NULL,
  PRIMARY KEY (from_typology, from_code, to_typology, to_code)
);
CREATE INDEX IF NOT EXISTS idx_crosswalk_to
  ON habitat_type_crosswalk(to_typology, to_code);

CREATE TABLE IF NOT EXISTS syntaxon (
  id        TEXT PRIMARY KEY,
  rank      TEXT NOT NULL,
  name      TEXT NOT NULL,
  author    TEXT NOT NULL DEFAULT '',
  parent_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS habitat_type_syntaxon (
  typology_id TEXT NOT NULL,
  code        TEXT NOT NULL,
  syntaxon_id TEXT NOT NULL,
  PRIMARY KEY (typology_id, code, syntaxon_id)
);
CREATE INDEX IF NOT EXISTS idx_hts_syntaxon ON habitat_type_syntaxon(syntaxon_id);

CREATE TABLE IF NOT EXISTS species_role (
  typology_id   TEXT NOT NULL,
  code          TEXT NOT NULL,
  concept_id    TEXT,
  verbatim_name TEXT NOT NULL,
  role          TEXT NOT NULL,
  fidelity      REAL,
  constancy     REAL,
  provenance    TEXT NOT NULL DEFAULT 'observed',
  derived_from  TEXT,
  PRIMARY KEY (typology_id, code, verbatim_name, role)
);
CREATE INDEX IF NOT EXISTS idx_species_role_concept ON species_role(concept_id);

CREATE TABLE IF NOT EXISTS localization (
  entity_type  TEXT NOT NULL,
  entity_key   TEXT NOT NULL,
  lang         TEXT NOT NULL,
  field        TEXT NOT NULL,
  value        TEXT NOT NULL,
  source       TEXT NOT NULL,
  provenance   TEXT NOT NULL,
  derived_from TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (entity_type, entity_key, lang, field, source)
);
CREATE INDEX IF NOT EXISTS idx_localization_lookup
  ON localization(entity_type, entity_key, lang);

-- Which areas a species concept occurs in. Absence of rows for a concept means
-- "unknown", not "does not occur" — the read side must keep those apart.
CREATE TABLE IF NOT EXISTS species_distribution (
  concept_id  TEXT NOT NULL,
  area_scheme TEXT NOT NULL,
  area_code   TEXT NOT NULL,
  PRIMARY KEY (concept_id, area_scheme, area_code)
);
CREATE INDEX IF NOT EXISTS idx_species_distribution_area
  ON species_distribution(area_scheme, area_code);

-- Pflanzenökologische Zeigerwerte (EIVE, Tichý, Midolo). Jede Zeile ist ein
-- Wert einer Dimension in einem Vokabular für ein Konzept; niche_width/
-- n_systems sind NULL, wenn das Vokabular sie nicht liefert (Tichý/Midolo
-- nie, EIVE immer) — nie 0/0.0.
CREATE TABLE IF NOT EXISTS trait_value (
  concept_id    TEXT NOT NULL,
  vocab         TEXT NOT NULL,
  vocab_version TEXT NOT NULL,
  dim           TEXT NOT NULL,
  value         REAL NOT NULL,
  niche_width   REAL,
  n_systems     INTEGER,
  PRIMARY KEY (concept_id, vocab, vocab_version, dim)
);
CREATE INDEX IF NOT EXISTS idx_trait_value_concept_id ON trait_value(concept_id);

-- Reines Ingest-Metadatum: wann wurde welche Vokabular-Version zuletzt
-- geschrieben. Kein fachlicher Inhalt für den Leser.
CREATE TABLE IF NOT EXISTS trait_vocabulary (
  vocab       TEXT NOT NULL,
  version     TEXT NOT NULL,
  ingested_at TEXT NOT NULL,
  PRIMARY KEY (vocab, version)
);

-- The WGSRPD area names (pipelines/wgsrpd). Pure overlay on the codes that
-- species_distribution already carries: no FK either way, and a code without a
-- row here stays a perfectly valid area — it just has no name yet.
CREATE TABLE IF NOT EXISTS area (
  area_scheme TEXT NOT NULL,
  area_code   TEXT NOT NULL,
  name_en     TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (area_scheme, area_code)
);

-- The prose descriptions from the EUNIS-ESy habitat factsheets
-- (pipelines/floraveg-factsheets). One row per described habitat type; a type
-- without a factsheet simply has none, which is the normal case.
CREATE TABLE IF NOT EXISTS habitat_description (
  typology_id    TEXT NOT NULL,
  code           TEXT NOT NULL,
  description_en TEXT NOT NULL,
  source         TEXT NOT NULL DEFAULT '',
  -- official (an external source's own wording) or situs (written by situs
  -- from its sources). Defaults to official because that is what every row
  -- written before this column existed was.
  provenance     TEXT NOT NULL DEFAULT 'official'
                 CHECK (provenance IN ('official', 'situs')),
  PRIMARY KEY (typology_id, code)
);
