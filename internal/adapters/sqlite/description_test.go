package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

var r22key = domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}

func writeDescription(t *testing.T, db *DB, d domain.HabitatDescription) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.UpsertDescription(d); err != nil {
		t.Fatalf("UpsertDescription: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

func TestDescription_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	want := domain.HabitatDescription{
		Key:    r22key,
		TextEN: "Meadows of lowland and montane areas.",
		Source: "floraveg:factsheets:2021-06-01",
	}
	writeDescription(t, db, want)

	got, err := db.Description(context.Background(), r22key)
	if err != nil {
		t.Fatalf("Description: %v", err)
	}
	if got != want {
		t.Errorf("description = %+v, want %+v", got, want)
	}
}

// A habitat type without a factsheet is the normal case — 264 of 7937 types
// carry one. That has to be a clean "not found", not an empty-but-present row.
func TestDescription_AbsentIsNotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Description(context.Background(), r22key)
	if !errors.Is(err, output.ErrNotFound) {
		t.Errorf("Description of an undescribed type = %v, want ErrNotFound", err)
	}
}

// Same key in another typology is another habitat type entirely.
func TestDescription_IsScopedToTheTypology(t *testing.T) {
	db := openTestDB(t)
	writeDescription(t, db, domain.HabitatDescription{Key: r22key, TextEN: "Meadows."})

	_, err := db.Description(context.Background(),
		domain.HabitatTypeKey{Typology: "eunis@2012", Code: "R22"})
	if !errors.Is(err, output.ErrNotFound) {
		t.Errorf("Description in another typology = %v, want ErrNotFound", err)
	}
}

func TestUpsertDescription_IsIdempotentAndReplacesTheText(t *testing.T) {
	db := openTestDB(t)
	writeDescription(t, db, domain.HabitatDescription{Key: r22key, TextEN: "Old.", Source: "a"})
	writeDescription(t, db, domain.HabitatDescription{Key: r22key, TextEN: "New.", Source: "b"})

	got, err := db.Description(context.Background(), r22key)
	if err != nil {
		t.Fatalf("Description: %v", err)
	}
	if got.TextEN != "New." || got.Source != "b" {
		t.Errorf("description = %+v, want the re-ingested text and source", got)
	}
}

func TestUpsertDescription_WriteErrorIsReturned(t *testing.T) {
	db := openTestDB(t)
	tx, err := db.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	err = tx.UpsertDescription(domain.HabitatDescription{Key: r22key, TextEN: "Meadows."})
	if err == nil {
		t.Fatal("UpsertDescription on a rolled-back transaction = nil error, want an error")
	}
	if !strings.HasPrefix(err.Error(), "sqlite: ") {
		t.Errorf("error = %q, want the adapter's own context prefixed", err)
	}
}

func TestDescription_QueryErrorIsReturned(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := db.Description(context.Background(), r22key)
	if err == nil || errors.Is(err, output.ErrNotFound) {
		t.Errorf("Description on a closed database = %v, want a real error", err)
	}
	if !strings.HasPrefix(err.Error(), "sqlite: ") {
		t.Errorf("error = %q, want the adapter's own context prefixed", err)
	}
}
