package hostus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// errBodyLimit bounds how much of a non-200 response body is quoted in the
// error, so a hostus 400 ("unknown entry_backbone") is distinguishable from
// any other 400 without risking an unbounded read.
const errBodyLimit = 512

// batchSize caps how many names go into one /v1/match request — hostus is a
// network hop, and 13791 species rows resolve to far fewer distinct names,
// but even that distinct set is chunked to keep any single request bounded.
const batchSize = 500

// Client resolves verbatim species names to hostus concept IDs. It is used
// at ingest time only; situs is autark for concept-ID queries at runtime.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a Client against baseURL, using httpClient for requests.
// A trailing slash is trimmed so baseURL+"/v1/match" never doubles up into
// "//v1/match" (a 404 whose cause a caller would otherwise have to guess).
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimSuffix(baseURL, "/"), http: httpClient}
}

type matchRequestName struct {
	ID       string `json:"id"`
	Verbatim string `json:"verbatim"`
}

type matchRequest struct {
	Names         []matchRequestName `json:"names"`
	EntryBackbone string             `json:"entry_backbone"`
}

type matchResult struct {
	ID        string `json:"id"`
	ConceptID string `json:"concept_id"`
	MatchType string `json:"match_type"`
}

type matchResponse struct {
	Results []matchResult `json:"results"`
}

// Resolve maps verbatim names to hostus concept IDs. The returned map omits
// any name hostus could not resolve — an absent key means unresolvable, not
// an empty-string concept id. names are deduplicated by the caller; Resolve
// itself just batches and posts whatever it is given.
func (c *Client) Resolve(ctx context.Context, names []string) (map[string]string, error) {
	resolved := make(map[string]string, len(names))

	for start := 0; start < len(names); start += batchSize {
		end := min(start+batchSize, len(names))
		batch := names[start:end]

		results, err := c.resolveBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		for _, r := range results {
			if r.ConceptID == "" {
				continue
			}
			idx, err := strconv.Atoi(r.ID)
			if err != nil || idx < 0 || idx >= len(batch) {
				return nil, fmt.Errorf("hostus: result id %q does not map to a sent name", r.ID)
			}
			resolved[batch[idx]] = r.ConceptID
		}
	}
	return resolved, nil
}

func (c *Client) resolveBatch(ctx context.Context, batch []string) ([]matchResult, error) {
	req := matchRequest{EntryBackbone: "wcvp"}
	req.Names = make([]matchRequestName, len(batch))
	for i, name := range batch {
		req.Names[i] = matchRequestName{ID: strconv.Itoa(i), Verbatim: name}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding hostus match request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/match", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building hostus match request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling hostus /v1/match: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit))
		return nil, fmt.Errorf("hostus /v1/match: unexpected status %s: %s", resp.Status, snippet)
	}

	var out matchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding hostus match response: %w", err)
	}
	return out.Results, nil
}
