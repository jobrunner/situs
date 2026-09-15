package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// seedAreaIndex fills both halves the area list joins: the names (area) and
// the distribution rows that decide which of them have data.
func seedAreaIndex(t *testing.T, db *DB, names map[string]string, distribution map[string][]string) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	for code, name := range names {
		if err := tx.UpsertArea(domain.NamedArea{
			Area:   domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: code},
			NameEN: name,
		}); err != nil {
			t.Fatalf("UpsertArea(%s): %v", code, err)
		}
	}
	for conceptID, codes := range distribution {
		for _, code := range codes {
			if err := tx.UpsertDistribution(conceptID,
				domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: code}); err != nil {
				t.Fatalf("UpsertDistribution(%s, %s): %v", conceptID, code, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// The list is exactly the codes the index has distribution data for, named
// from the area table — a named area nobody occurs in is not in it.
func TestAreasWithData_ListsOnlyOccupiedAreasWithTheirNames(t *testing.T) {
	db := openTestDB(t)
	seedAreaIndex(t,
		db,
		map[string]string{"GER": "Germany", "FRA": "France", "NZN": "New Zealand North"},
		map[string][]string{"wcvp:concept:1": {"GER", "FRA"}, "wcvp:concept:2": {"GER"}},
	)

	got, err := db.AreasWithData(context.Background(), domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("AreasWithData: %v", err)
	}
	want := []domain.NamedArea{
		{Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "FRA"}, NameEN: "France"},
		{Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}, NameEN: "Germany"},
	}
	if len(got) != len(want) {
		t.Fatalf("areas = %+v, want %+v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("areas[%d] = %+v, want %+v (sorted by code)", i, got[i], w)
		}
	}
}

// A code with distribution data but no name row stays in the list with an
// empty name. Dropping it would hide data the index actually holds, and
// repeating the code as a pseudo-name would claim a name nobody ingested.
func TestAreasWithData_KeepsAnUnnamedAreaWithAnEmptyName(t *testing.T) {
	db := openTestDB(t)
	seedAreaIndex(t, db, map[string]string{"GER": "Germany"},
		map[string][]string{"wcvp:concept:1": {"GER", "XXX"}})

	got, err := db.AreasWithData(context.Background(), domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("AreasWithData: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("areas = %+v, want both GER and the unnamed XXX", got)
	}
	unnamed := got[1]
	if unnamed.Code != "XXX" || unnamed.NameEN != "" {
		t.Errorf("areas[1] = %+v, want code XXX with an empty name", unnamed)
	}
}

// Scheme is half the identity of an area: a code stored under another scheme
// must not leak into the wgsrpd_l3 answer.
func TestAreasWithData_IsScopedToTheScheme(t *testing.T) {
	db := openTestDB(t)
	seedAreaIndex(t, db, map[string]string{"GER": "Germany"},
		map[string][]string{"wcvp:concept:1": {"GER"}})

	got, err := db.AreasWithData(context.Background(), "some_other_scheme")
	if err != nil {
		t.Fatalf("AreasWithData: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("areas = %+v, want none — the data is in another scheme", got)
	}
}

// Re-ingesting a repinned artifact must replace the name, not fail and not
// leave the old one standing.
func TestUpsertArea_IsIdempotentAndUpdatesTheName(t *testing.T) {
	db := openTestDB(t)
	seedAreaIndex(t, db, map[string]string{"GER": "Germany"},
		map[string][]string{"wcvp:concept:1": {"GER"}})
	seedAreaIndex(t, db, map[string]string{"GER": "Deutschland"}, nil)

	got, err := db.AreasWithData(context.Background(), domain.SchemeWGSRPDL3)
	if err != nil {
		t.Fatalf("AreasWithData: %v", err)
	}
	if len(got) != 1 || got[0].NameEN != "Deutschland" {
		t.Errorf("areas = %+v, want the re-ingested name", got)
	}
}

func TestUpsertArea_WriteErrorIsReturned(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	err = tx.UpsertArea(domain.NamedArea{
		Area: domain.Area{Scheme: domain.SchemeWGSRPDL3, Code: "GER"}, NameEN: "Germany",
	})
	if err == nil {
		t.Fatal("UpsertArea on a rolled-back transaction = nil error, want an error")
	}
	if !strings.HasPrefix(err.Error(), "sqlite: ") {
		t.Errorf("error = %q, want the adapter's own context prefixed", err)
	}
}

func TestAreasWithData_QueryErrorIsReturned(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := db.AreasWithData(context.Background(), domain.SchemeWGSRPDL3); err == nil {
		t.Error("AreasWithData on a closed database = nil error, want an error")
	}
}

func TestAreasWithData_RowsIterationAndScanErrorsAreReturned(t *testing.T) {
	ctx := context.Background()
	for mode, want := range map[stubMode]string{
		stubModeRowsErr: "iterating areas",
		stubModeScanErr: "scanning area",
	} {
		_, err := (&DB{DB: newStubDB(t, mode)}).AreasWithData(ctx, domain.SchemeWGSRPDL3)
		if err == nil {
			t.Fatalf("AreasWithData in mode %v = nil error, want an error", mode)
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name %q", err, want)
		}
	}
}
