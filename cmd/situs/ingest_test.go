package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIngestCSV(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func seedIngestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeIngestCSV(t, dir, "typologies.csv",
		"id,scheme,version,name,source_ref\neunis@2021,eunis,2021,EUNIS 2021,https://example.org\n")
	writeIngestCSV(t, dir, "habitat_types.csv",
		"typology_id,code,level,name_en,parent_code,priority\neunis@2021,R22,3,Hay meadow,R2,\n")
	writeIngestCSV(t, dir, "crosswalks.csv",
		"from_typology,from_code,to_typology,to_code,qualifier\n")
	writeIngestCSV(t, dir, "syntaxa.csv", "id,rank,name,parent_id\n")
	writeIngestCSV(t, dir, "habitat_type_syntaxa.csv", "typology_id,code,syntaxon_id\n")
	return dir
}

func TestIngestCommandLoadsCSVsAndPrintsTheReport(t *testing.T) {
	csvDir := seedIngestDir(t)
	dbPath := filepath.Join(t.TempDir(), "situs.db")

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"ingest", "--csv-dir", csvDir, "--db", dbPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("executing ingest: %v", err)
	}
	if !strings.Contains(out.String(), `"HabitatTypes": 1`) {
		t.Errorf("output = %q, want it to report one habitat type", out.String())
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("sqlite index was not created: %v", err)
	}
}

func TestIngestCommandFailsOnAMissingCSVDirectory(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"ingest", "--csv-dir", filepath.Join(t.TempDir(), "missing"), "--db", filepath.Join(t.TempDir(), "situs.db")})

	if err := root.Execute(); err == nil {
		t.Fatal("executing ingest with a missing csv-dir = nil error, want an error")
	}
}

func TestIngestCommandFailsOnAnUnreachableDB(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"ingest", "--csv-dir", seedIngestDir(t), "--db", t.TempDir()})

	if err := root.Execute(); err == nil {
		t.Fatal("executing ingest with an unreachable db path = nil error, want an error")
	}
}

func TestIngestCommandRequiresBothFlags(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"ingest"})

	if err := root.Execute(); err == nil {
		t.Fatal("executing ingest without flags = nil error, want an error")
	}
}
