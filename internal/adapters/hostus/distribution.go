package hostus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jobrunner/situs/internal/domain"
	"github.com/jobrunner/situs/internal/ports/output"
)

// conceptResponse is the slice of GET /v1/concept/{id} this adapter needs.
type conceptResponse struct {
	Distribution []struct {
		AreaScheme string `json:"area_scheme"`
		AreaCode   string `json:"area_code"`
	} `json:"distribution"`
}

// Areas asks hostus once per concept: there is no batch route for distribution
// (/v1/match carries none), so the request count equals the concept count. The
// caller paces this — hostus rate-limits.
func (c *Client) Areas(ctx context.Context, conceptIDs []string) (map[string][]domain.Area, error) {
	out := map[string][]domain.Area{}
	for _, id := range conceptIDs {
		areas, err := c.areasOf(ctx, id)
		if err != nil {
			return nil, err
		}
		if len(areas) > 0 {
			out[id] = areas
		}
	}
	return out, nil
}

func (c *Client) areasOf(ctx context.Context, conceptID string) ([]domain.Area, error) {
	endpoint := c.baseURL + "/v1/concept/" + url.PathEscape(conceptID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building the hostus concept request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling hostus %s: %w: %w", endpoint, output.ErrResolverUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// A concept hostus does not know has no areas — that is data, not failure.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit))
		return nil, fmt.Errorf("hostus answered %s for %s: %w: %s",
			resp.Status, conceptID, output.ErrResolverUnavailable, snippet)
	}

	var body conceptResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding the hostus concept response: %w", err)
	}
	areas := make([]domain.Area, 0, len(body.Distribution))
	for _, d := range body.Distribution {
		// Only the scheme situs stores; anything else would be a code in a
		// namespace the index cannot compare against.
		if d.AreaScheme != domain.SchemeWGSRPDL3 {
			continue
		}
		a := domain.Area{Scheme: d.AreaScheme, Code: d.AreaCode}
		if a.IsComplete() {
			areas = append(areas, a)
		}
	}
	return areas, nil
}
