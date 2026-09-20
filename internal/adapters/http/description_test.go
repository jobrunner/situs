package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The description reaches the wire on the detail route — there is no separate
// endpoint for it, deliberately: a client that wants the habitat wants its
// description in the same answer.
func TestHabitatTypeDetail_CarriesDescriptionAndItsGermanOverlay(t *testing.T) {
	rec := serve(t, newTestServer(t, seededQueryService()), http.MethodGet,
		"/v1/habitat-type/eunis@2021/R22?lang=de")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Description *struct {
			Value      string `json:"value"`
			Provenance string `json:"provenance"`
			Source     string `json:"source"`
		} `json:"description"`
		DescriptionDE *struct {
			Value      string `json:"value"`
			Provenance string `json:"provenance"`
			Source     string `json:"source"`
		} `json:"description_de"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding body %q: %v", rec.Body, err)
	}
	if got.Description == nil || got.Description.Value != "Hay meadows of lowland and montane areas." {
		t.Fatalf("description = %+v, want the English factsheet text", got.Description)
	}
	if got.Description.Provenance != "official" {
		t.Errorf("provenance = %q, want official for the factsheet wording", got.Description.Provenance)
	}
	if got.DescriptionDE == nil || got.DescriptionDE.Value != "Mähwiesen der Tief- und mittleren Lagen." {
		t.Fatalf("description_de = %+v, want the German overlay", got.DescriptionDE)
	}
	if got.DescriptionDE.Provenance != "situs" {
		t.Errorf("provenance = %q, want situs — nobody outside situs vouches for the translation", got.DescriptionDE.Provenance)
	}
}

// A type without a factsheet must omit both fields rather than send empty
// ones: an empty string claims a description exists and says nothing.
func TestHabitatTypeDetail_OmitsDescriptionWhenThereIsNone(t *testing.T) {
	q := seededQueryService()
	q.undescribed = true

	rec := serve(t, newTestServer(t, q), http.MethodGet, "/v1/habitat-type/eunis@2021/R22?lang=de")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, field := range []string{`"description"`, `"description_de"`} {
		if strings.Contains(body, field) {
			t.Errorf("body carries %s although the type has none: %s", field, body)
		}
	}
}
