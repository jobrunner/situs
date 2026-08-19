package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/situs/internal/domain"
)

// A '=' crosswalk lends the official German Annex I name to the EUNIS type as
// an entry-level label — clearly marked as derived.
func TestDeriveGermanLabels_CopiesOfficialNameAcrossSameQualifier(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	repo.localizations = []domain.Localization{{
		EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de",
		Field: "name", Value: "Magere Flachland-Mähwiesen",
		Source: "ffh-richtlinie-de", Provenance: "official",
	}}

	n, err := DeriveGermanLabels(context.Background(), repo)
	if err != nil {
		t.Fatalf("DeriveGermanLabels: %v", err)
	}
	if n != 1 {
		t.Fatalf("derived %d labels, want 1", n)
	}
	got := repo.derivedFor("habitat_type", "eunis@2021:R22")
	if got.Value != "Magere Flachland-Mähwiesen" {
		t.Errorf("Value = %q, want the official Annex I name", got.Value)
	}
	if got.Provenance != "derived" {
		t.Errorf("Provenance = %q, want %q — a derived label must never pose as official", got.Provenance, "derived")
	}
	if got.DerivedFrom != "annex1:6510 qualifier==" {
		t.Errorf("DerivedFrom = %q, want %q", got.DerivedFrom, "annex1:6510 qualifier==")
	}
}

// '<', '>' and '#' are too imprecise to lend a name.
func TestDeriveGermanLabels_IgnoresNonSameQualifiers(t *testing.T) {
	for _, q := range []domain.Qualifier{
		domain.QualifierNarrower, domain.QualifierBroader, domain.QualifierPartial,
		// '≈' is ingested (it carries real correspondences) but must lend no name.
		domain.QualifierApproximate,
	} {
		repo := newFakeRepo()
		repo.crosswalks = []domain.Crosswalk{{
			From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
			To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
			Qualifier: q,
		}}
		repo.localizations = []domain.Localization{{
			EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de",
			Field: "name", Value: "Magere Flachland-Mähwiesen",
			Source: "ffh-richtlinie-de", Provenance: "official",
		}}

		n, err := DeriveGermanLabels(context.Background(), repo)
		if err != nil {
			t.Fatalf("DeriveGermanLabels(%q): %v", q, err)
		}
		if n != 0 {
			t.Errorf("qualifier %q derived %d labels, want 0", q, n)
		}
	}
}

// An existing official/curated German name must never be overwritten by a
// derived one.
func TestDeriveGermanLabels_DoesNotOverrideAnExistingOfficialName(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	repo.localizations = []domain.Localization{
		{EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de", Field: "name",
			Value: "Magere Flachland-Mähwiesen", Source: "ffh-richtlinie-de", Provenance: "official"},
		{EntityType: "habitat_type", EntityKey: "eunis@2021:R22", Lang: "de", Field: "name",
			Value: "Kuratierter Name", Source: "curated", Provenance: "curated"},
	}

	n, err := DeriveGermanLabels(context.Background(), repo)
	if err != nil {
		t.Fatalf("DeriveGermanLabels: %v", err)
	}
	if got := repo.localizationFor("habitat_type", "eunis@2021:R22", "curated"); got.Value != "Kuratierter Name" {
		t.Errorf("curated value = %q, want it untouched", got.Value)
	}
	// The two assertions above pass even if derivation ran anyway (fakeRepo
	// only appends, so a stray derived row would not touch the curated row)
	// — these two additionally prove no derivation happened at all.
	if n != 0 {
		t.Errorf("n = %d, want 0 — an existing curated name must block derivation entirely", n)
	}
	if got := repo.localizationFor("habitat_type", "eunis@2021:R22", "derived-annex1"); got != (domain.Localization{}) {
		t.Errorf("derivedFor = %+v, want the zero value — no derived-annex1 row must exist", got)
	}
}

// A missing localizations.csv is "no localizations", not an error — no
// source in this foundation produces that file yet.
func TestIngestLocalizations_MissingFileIsNotAnError(t *testing.T) {
	repo := newFakeRepo()
	path := filepath.Join(t.TempDir(), "localizations.csv")

	n, err := IngestLocalizations(context.Background(), repo, path)
	if err != nil {
		t.Fatalf("IngestLocalizations: %v", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
	if repo.committed {
		t.Error("a missing file must not open/commit a transaction")
	}
}

func TestIngestLocalizations_ReadsEveryColumn(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "localizations.csv",
		"entity_type,entity_key,lang,field,value,source,provenance\n"+
			"habitat_type,annex1:6510,de,name,Magere Flachland-Mähwiesen,ffh-richtlinie-de,official\n")

	repo := newFakeRepo()
	n, err := IngestLocalizations(context.Background(), repo, filepath.Join(dir, "localizations.csv"))
	if err != nil {
		t.Fatalf("IngestLocalizations: %v", err)
	}
	if n != 1 {
		t.Fatalf("n = %d, want 1", n)
	}
	if len(repo.localizations) != 1 {
		t.Fatalf("stored %d localizations, want 1", len(repo.localizations))
	}
	got := repo.localizations[0]
	want := domain.Localization{
		EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de", Field: "name",
		Value: "Magere Flachland-Mähwiesen", Source: "ffh-richtlinie-de", Provenance: "official",
	}
	if got != want {
		t.Errorf("localization = %+v, want %+v", got, want)
	}
	if !repo.committed {
		t.Error("IngestLocalizations did not commit")
	}
}

// provenance comes from the file and must be one of the three legal values;
// anything else is a malformed row like any other.
func TestIngestLocalizations_RejectsIllegalProvenance(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "localizations.csv",
		"entity_type,entity_key,lang,field,value,source,provenance\n"+
			"habitat_type,annex1:6510,de,name,Magere Flachland-Mähwiesen,ffh-richtlinie-de,official\n"+
			"habitat_type,annex1:6520,de,name,Kalk-Pionierrasen,unknown,guessed\n")

	repo := newFakeRepo()
	n, err := IngestLocalizations(context.Background(), repo, filepath.Join(dir, "localizations.csv"))
	if err != nil {
		t.Fatalf("IngestLocalizations: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1 (the illegal-provenance row is skipped)", n)
	}
}

func TestIngestLocalizations_SkipsShortRow(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "localizations.csv",
		"entity_type,entity_key,lang,field,value,source,provenance\n"+
			"habitat_type,annex1:6510,de,name,Magere Flachland-Mähwiesen,ffh-richtlinie-de,official\n"+
			"habitat_type,annex1:6520,de,name\n") // truncated

	repo := newFakeRepo()
	n, err := IngestLocalizations(context.Background(), repo, filepath.Join(dir, "localizations.csv"))
	if err != nil {
		t.Fatalf("IngestLocalizations: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1", n)
	}
}

func TestIngestLocalizations_MissingRequiredColumnFails(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "localizations.csv",
		"entity_type,entity_key,lang,field,value,source\n"+ // no "provenance" column
			"habitat_type,annex1:6510,de,name,Magere Flachland-Mähwiesen,ffh-richtlinie-de\n")

	repo := newFakeRepo()
	if _, err := IngestLocalizations(context.Background(), repo, filepath.Join(dir, "localizations.csv")); err == nil {
		t.Fatal("IngestLocalizations with a missing required column = nil error, want an error")
	}
}

func TestIngestLocalizations_BeginErrorIsReturned(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "localizations.csv",
		"entity_type,entity_key,lang,field,value,source,provenance\n"+
			"habitat_type,annex1:6510,de,name,Magere Flachland-Mähwiesen,ffh-richtlinie-de,official\n")

	repo := newFakeRepo()
	repo.beginErr = fmt.Errorf("boom")
	if _, err := IngestLocalizations(context.Background(), repo, filepath.Join(dir, "localizations.csv")); err == nil {
		t.Fatal("IngestLocalizations with a Begin error = nil error, want an error")
	}
}

func TestIngestLocalizations_CommitErrorIsReturned(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "localizations.csv",
		"entity_type,entity_key,lang,field,value,source,provenance\n"+
			"habitat_type,annex1:6510,de,name,Magere Flachland-Mähwiesen,ffh-richtlinie-de,official\n")

	repo := newFakeRepo()
	repo.commitErr = fmt.Errorf("disk full")
	if _, err := IngestLocalizations(context.Background(), repo, filepath.Join(dir, "localizations.csv")); err == nil {
		t.Fatal("IngestLocalizations with a Commit error = nil error, want an error")
	}
}

func TestIngestLocalizations_RepositoryErrorRollsBackAndReturnsTheError(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "localizations.csv",
		"entity_type,entity_key,lang,field,value,source,provenance\n"+
			"habitat_type,annex1:6510,de,name,Magere Flachland-Mähwiesen,ffh-richtlinie-de,official\n")

	repo := newFakeRepo()
	repo.failOn = "UpsertLocalization"
	if _, err := IngestLocalizations(context.Background(), repo, filepath.Join(dir, "localizations.csv")); err == nil {
		t.Fatal("IngestLocalizations with a repository error = nil error, want an error")
	}
	if repo.committed {
		t.Error("ingest committed despite a repository error")
	}
	if !repo.rolledBack {
		t.Error("ingest did not roll back after a repository error")
	}
}

func TestIngestLocalizations_RollbackErrorIsWrappedWithTheOriginal(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, dir, "localizations.csv",
		"entity_type,entity_key,lang,field,value,source,provenance\n"+
			"habitat_type,annex1:6510,de,name,Magere Flachland-Mähwiesen,ffh-richtlinie-de,official\n")

	repo := newFakeRepo()
	repo.failOn = "UpsertLocalization"
	repo.rollbackErr = fmt.Errorf("connection lost")
	_, err := IngestLocalizations(context.Background(), repo, filepath.Join(dir, "localizations.csv"))
	if err == nil {
		t.Fatal("IngestLocalizations = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error = %q, want it to mention the rollback failure too", err)
	}
}

// os.Stat can fail for a reason other than "does not exist" (e.g. a
// permission error) — that must surface as a real error, not silently be
// treated like a missing file.
func TestIngestLocalizations_StatErrorOtherThanNotExistFails(t *testing.T) {
	dir := t.TempDir()
	// A path with a non-directory component in the middle produces ENOTDIR,
	// not ENOENT.
	writeCSV(t, dir, "not-a-dir", "x")
	path := filepath.Join(dir, "not-a-dir", "localizations.csv")

	repo := newFakeRepo()
	if _, err := IngestLocalizations(context.Background(), repo, path); err == nil {
		t.Fatal("IngestLocalizations with a Stat error = nil error, want an error")
	}
}

func TestDeriveGermanLabels_CrosswalksToErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalksToErr = fmt.Errorf("boom")
	if _, err := DeriveGermanLabels(context.Background(), repo); err == nil {
		t.Fatal("DeriveGermanLabels with a CrosswalksTo error = nil error, want an error")
	}
}

func TestDeriveGermanLabels_BeginErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.beginErr = fmt.Errorf("boom")
	if _, err := DeriveGermanLabels(context.Background(), repo); err == nil {
		t.Fatal("DeriveGermanLabels with a Begin error = nil error, want an error")
	}
}

func TestDeriveGermanLabels_CommitErrorIsReturned(t *testing.T) {
	repo := newFakeRepo()
	repo.commitErr = fmt.Errorf("disk full")
	if _, err := DeriveGermanLabels(context.Background(), repo); err == nil {
		t.Fatal("DeriveGermanLabels with a Commit error = nil error, want an error")
	}
}

func TestDeriveGermanLabels_LocalizationErrorRollsBackAndReturnsTheError(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	repo.localizationErr = fmt.Errorf("boom")

	if _, err := DeriveGermanLabels(context.Background(), repo); err == nil {
		t.Fatal("DeriveGermanLabels with a Localization error = nil error, want an error")
	}
	if repo.committed {
		t.Error("derivation committed despite a repository error")
	}
	if !repo.rolledBack {
		t.Error("derivation did not roll back after a repository error")
	}
}

// The Annex I target's own lookup (the second Localization call, after the
// source check passed) can fail independently of the first.
func TestDeriveGermanLabels_TargetLocalizationErrorRollsBackAndReturnsTheError(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	repo.localizationErrOnCall = 2 // 1st call checks the source, 2nd fetches the target

	if _, err := DeriveGermanLabels(context.Background(), repo); err == nil {
		t.Fatal("DeriveGermanLabels with a target Localization error = nil error, want an error")
	}
	if !repo.rolledBack {
		t.Error("derivation did not roll back after a repository error")
	}
}

// A repository error mid-loop that also fails to roll back must wrap both.
func TestDeriveGermanLabels_LoopErrorRollbackErrorIsWrappedWithTheOriginal(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	repo.localizationErr = fmt.Errorf("boom")
	repo.rollbackErr = fmt.Errorf("connection lost")

	_, err := DeriveGermanLabels(context.Background(), repo)
	if err == nil {
		t.Fatal("DeriveGermanLabels = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error = %q, want it to mention the rollback failure too", err)
	}
}

func TestDeriveGermanLabels_UpsertErrorRollsBackAndReturnsTheError(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	repo.localizations = []domain.Localization{{
		EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de",
		Field: "name", Value: "Magere Flachland-Mähwiesen",
		Source: "ffh-richtlinie-de", Provenance: "official",
	}}
	repo.failOn = "UpsertLocalization"

	if _, err := DeriveGermanLabels(context.Background(), repo); err == nil {
		t.Fatal("DeriveGermanLabels with an Upsert error = nil error, want an error")
	}
	if !repo.rolledBack {
		t.Error("derivation did not roll back after a repository error")
	}
}

// A target with no official/curated name (e.g. only a derived one, which
// must never itself seed another derivation) yields nothing to copy.
func TestDeriveGermanLabels_SkipsWhenTargetHasNoOfficialOrCuratedName(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}

	n, err := DeriveGermanLabels(context.Background(), repo)
	if err != nil {
		t.Fatalf("DeriveGermanLabels: %v", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0 (no source label to copy)", n)
	}
}

// When an Annex I target carries both an official and a curated de/name, the
// official wording must win deterministically — not whichever happens to
// come first from the repository.
func TestDeriveGermanLabels_PrefersOfficialOverCuratedOnTheTarget(t *testing.T) {
	repo := newFakeRepo()
	repo.crosswalks = []domain.Crosswalk{{
		From:      domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"},
		To:        domain.HabitatTypeKey{Typology: "annex1", Code: "6510"},
		Qualifier: domain.QualifierSame,
	}}
	// Curated listed first: a naive "first match" would copy this one.
	repo.localizations = []domain.Localization{
		{EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de", Field: "name",
			Value: "Kuratierter Name", Source: "curated", Provenance: "curated"},
		{EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de", Field: "name",
			Value: "Magere Flachland-Mähwiesen", Source: "ffh-richtlinie-de", Provenance: "official"},
	}

	if _, err := DeriveGermanLabels(context.Background(), repo); err != nil {
		t.Fatalf("DeriveGermanLabels: %v", err)
	}
	got := repo.localizationFor("habitat_type", "eunis@2021:R22", "derived-annex1")
	if got.Value != "Magere Flachland-Mähwiesen" {
		t.Errorf("Value = %q, want the official name preferred over the curated one", got.Value)
	}
}

// Two '=' crosswalks from the same source type to different Annex I targets
// must not both land a derived-annex1 row: the second write would silently
// replace the first at the same (entity, field, source) slot, and a count
// that includes both would overstate what actually survived.
func TestDeriveGermanLabels_FirstCrosswalkWinsWhenSourceHasTwoSameQualifierTargets(t *testing.T) {
	repo := newFakeRepo()
	from := domain.HabitatTypeKey{Typology: "eunis@2021", Code: "R22"}
	repo.crosswalks = []domain.Crosswalk{
		{From: from, To: domain.HabitatTypeKey{Typology: "annex1", Code: "6510"}, Qualifier: domain.QualifierSame},
		{From: from, To: domain.HabitatTypeKey{Typology: "annex1", Code: "6520"}, Qualifier: domain.QualifierSame},
	}
	repo.localizations = []domain.Localization{
		{EntityType: "habitat_type", EntityKey: "annex1:6510", Lang: "de", Field: "name",
			Value: "Erster Name", Source: "ffh-richtlinie-de", Provenance: "official"},
		{EntityType: "habitat_type", EntityKey: "annex1:6520", Lang: "de", Field: "name",
			Value: "Zweiter Name", Source: "ffh-richtlinie-de", Provenance: "official"},
	}

	n, err := DeriveGermanLabels(context.Background(), repo)
	if err != nil {
		t.Fatalf("DeriveGermanLabels: %v", err)
	}
	if n != 1 {
		t.Errorf("n = %d, want 1 (only the first crosswalk's target wins)", n)
	}
	derived := 0
	for _, l := range repo.localizations {
		if l.Source == "derived-annex1" {
			derived++
		}
	}
	if derived != 1 {
		t.Errorf("stored %d derived-annex1 rows, want 1 (count must match what actually survives)", derived)
	}
	if got := repo.localizationFor("habitat_type", "eunis@2021:R22", "derived-annex1"); got.Value != "Erster Name" {
		t.Errorf("derived value = %q, want the first crosswalk's target name", got.Value)
	}
}
