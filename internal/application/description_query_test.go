package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/input"
)

// seedDescribedType gives the repo one habitat type plus its description.
func seedDescribedType(t *testing.T, text string) (*fakeRepo, domain.HabitatTypeKey) {
	t.Helper()
	key := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo := newFakeRepo()
	repo.typologies = append(repo.typologies, domain.Typology{ID: "eunis@2021", Scheme: "eunis", Version: "2021"})
	repo.types = append(repo.types, domain.HabitatType{Key: key, NameEN: "Hay meadow"})
	repo.descriptions = append(repo.descriptions, domain.HabitatDescription{
		Key: key, TextEN: text, Source: "floraveg:2021-06-01",
	})
	return repo, key
}

func TestHabitatType_CarriesTheDescription(t *testing.T) {
	repo, key := seedDescribedType(t, "Hay meadows of lowland and montane areas.")

	got, err := NewQueryService(repo).HabitatType(context.Background(), key, "", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.Description != "Hay meadows of lowland and montane areas." {
		t.Errorf("description = %q, want the ingested text", got.Description)
	}
	if got.DescriptionDE != nil {
		t.Errorf("description_de = %+v, want it absent without ?lang=de", got.DescriptionDE)
	}
}

// 264 of 7937 types carry a factsheet. For the rest the field is ABSENT, not
// empty: an empty string would claim a description exists and says nothing.
func TestHabitatType_WithoutDescriptionLeavesTheFieldEmpty(t *testing.T) {
	repo, key := seedDescribedType(t, "Hay meadows.")
	repo.descriptions = nil

	got, err := NewQueryService(repo).HabitatType(context.Background(), key, "", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.Description != "" || got.DescriptionDE != nil {
		t.Errorf("description = %q / %+v, want both absent", got.Description, got.DescriptionDE)
	}
}

// The German description is an overlay of the same shape as name_de: the
// English text stays the identity, the translation carries its provenance.
func TestHabitatType_GermanDescriptionIsAnOverlay(t *testing.T) {
	repo, key := seedDescribedType(t, "Hay meadows of lowland areas.")
	repo.localizations = append(repo.localizations, domain.Localization{
		EntityType: "habitat_type", EntityKey: key.String(), Lang: "de",
		Field: "description", Value: "Mähwiesen der Tieflagen.",
		Source: "situs@0.7.0", Provenance: "situs",
	})

	got, err := NewQueryService(repo).HabitatType(context.Background(), key, "de", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.Description != "Hay meadows of lowland areas." {
		t.Errorf("description = %q, want the English text to stay the identity", got.Description)
	}
	if got.DescriptionDE == nil {
		t.Fatal("description_de is absent, want the German overlay")
	}
	if got.DescriptionDE.Value != "Mähwiesen der Tieflagen." ||
		got.DescriptionDE.Provenance != "situs" || got.DescriptionDE.Source != "situs@0.7.0" {
		t.Errorf("description_de = %+v, want value, provenance and source of the row", got.DescriptionDE)
	}
}

// Same ranking as the name overlay: the most authoritative wording wins, and
// the answer must not depend on row order.
func TestHabitatType_GermanDescriptionPrefersTheMostAuthoritativeRow(t *testing.T) {
	repo, key := seedDescribedType(t, "Hay meadows.")
	for _, l := range []domain.Localization{
		{Field: "description", Value: "von situs", Provenance: "situs", Source: "situs@0.7.0"},
		{Field: "description", Value: "amtlich", Provenance: "official", Source: "eur-lex"},
	} {
		l.EntityType, l.EntityKey, l.Lang = "habitat_type", key.String(), "de"
		repo.localizations = append(repo.localizations, l)
	}

	got, err := NewQueryService(repo).HabitatType(context.Background(), key, "de", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.DescriptionDE.Value != "amtlich" {
		t.Errorf("description_de = %+v, want the official row to win", got.DescriptionDE)
	}
}

// A German NAME must never be served as a description, and vice versa: they
// are different fields of the same entity and share one localization table.
func TestHabitatType_NameAndDescriptionOverlaysDoNotMix(t *testing.T) {
	repo, key := seedDescribedType(t, "Hay meadows.")
	repo.localizations = append(repo.localizations, domain.Localization{
		EntityType: "habitat_type", EntityKey: key.String(), Lang: "de",
		Field: "name", Value: "Mähwiese", Source: "situs@0.7.0", Provenance: "situs",
	})

	got, err := NewQueryService(repo).HabitatType(context.Background(), key, "de", input.AreaFilter{})
	if err != nil {
		t.Fatalf("HabitatType: %v", err)
	}
	if got.NameDE == nil || got.NameDE.Value != "Mähwiese" {
		t.Errorf("name_de = %+v, want the German name", got.NameDE)
	}
	if got.DescriptionDE != nil {
		t.Errorf("description_de = %+v, want it absent — only a name was localized", got.DescriptionDE)
	}
}

func TestHabitatType_DescriptionErrorIsReturned(t *testing.T) {
	repo, key := seedDescribedType(t, "Hay meadows.")
	repo.descriptionErr = errors.New("index unreadable")

	if _, err := NewQueryService(repo).HabitatType(context.Background(), key, "", input.AreaFilter{}); err == nil {
		t.Fatal("HabitatType with Description failing = nil error, want an error")
	}
}

// The German overlay is a second read. Its failure must surface, not silently
// serve the English text as if no translation existed.
func TestHabitatType_GermanDescriptionLookupErrorIsReturned(t *testing.T) {
	repo, key := seedDescribedType(t, "Hay meadows.")
	repo.localizationErrOnCall = 2 // the summary's label lookup is the first

	_, err := NewQueryService(repo).HabitatType(context.Background(), key, "de", input.AreaFilter{})
	if err == nil {
		t.Fatal("HabitatType with the German description lookup failing = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "German description") {
		t.Errorf("error = %q, want it to name the failing lookup", err)
	}
}
