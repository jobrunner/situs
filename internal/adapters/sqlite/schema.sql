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
