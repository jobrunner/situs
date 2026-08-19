package hostus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jobrunner/situs/internal/ports/output"
)

// errBodyLimit bounds how much of a non-200 response body is quoted in the
// error, so a hostus 400 ("unknown entry_backbone") is distinguishable from
// any other 400 without risking an unbounded read.
const errBodyLimit = 512

// DefaultBatchSize caps how many names go into one /v1/match request — hostus
// is a network hop, and 13791 species rows resolve to far fewer distinct names,
// but even that distinct set is chunked to keep any single request bounded.
//
// 50, not more: hostus applies a fixed 30s per-request timeout, and the cost of
// a batch scales with its content. Measured against the real ESy names and a
// full hostus index: 500 names exceeded the timeout (HTTP 500 after 30s), 200
// still did on the most expensive stretch of names, the worst 100-name window
// took 19.5s and the worst 50-name window 16.3s. 50 is the size that keeps a
// margin on real data — but a slower machine can need less, hence
// SITUS_HOSTUS_BATCH_SIZE and the downshift below.
const DefaultBatchSize = 50

// DefaultEntryBackbone is the hostus taxonomic backbone situs matches against.
// The pinned ESy species names are vascular plants, which is what wcvp covers;
// a hostus instance built on another backbone is configured, not recompiled.
const DefaultEntryBackbone = "wcvp"

// minBatchSize is the floor the downshift stops at. Below this, halving stops
// buying anything: the failure is then hostus being unreachable or broken, not
// the batch being too large.
const minBatchSize = 5

// Client resolves verbatim species names to hostus concept IDs. It is used
// at ingest time only; situs is autark for concept-ID queries at runtime.
type Client struct {
	baseURL       string
	http          *http.Client
	batchSize     int
	entryBackbone string
}

// NewClient builds a Client against baseURL, using httpClient for requests and
// batchSize names per request (DefaultBatchSize when <= 0) against the named
// entryBackbone (DefaultEntryBackbone when empty).
// A trailing slash is trimmed so baseURL+"/v1/match" never doubles up into
// "//v1/match" (a 404 whose cause a caller would otherwise have to guess).
func NewClient(baseURL string, httpClient *http.Client, batchSize int, entryBackbone string) *Client {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	if entryBackbone == "" {
		entryBackbone = DefaultEntryBackbone
	}
	return &Client{
		baseURL:       strings.TrimSuffix(baseURL, "/"),
		http:          httpClient,
		batchSize:     batchSize,
		entryBackbone: entryBackbone,
	}
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

	for start := 0; start < len(names); start += c.batchSize {
		end := min(start+c.batchSize, len(names))
		if err := c.resolveInto(ctx, names[start:end], resolved); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

// resolveInto resolves one batch into resolved, halving the batch and retrying
// when hostus could not answer it. The configured size is a default, not a
// cliff: hostus' per-request timeout is fixed and the cost of a batch depends on
// its content, so a batch that is too large for this machine must degrade
// instead of failing a whole ingest. A hostus-side rejection is deterministic
// and is never retried.
func (c *Client) resolveInto(ctx context.Context, batch []string, resolved map[string]string) error {
	results, err := c.resolveBatch(ctx, batch)
	if err != nil {
		if !errors.Is(err, output.ErrResolverUnavailable) || len(batch) <= minBatchSize {
			return err
		}
		half := len(batch) / 2
		slog.WarnContext(ctx, "hostus could not answer a batch, retrying at half size",
			"size", len(batch), "half", half, "error", err)
		if err := c.resolveInto(ctx, batch[:half], resolved); err != nil {
			return err
		}
		return c.resolveInto(ctx, batch[half:], resolved)
	}

	for _, r := range results {
		if r.ConceptID == "" {
			continue
		}
		idx, err := strconv.Atoi(r.ID)
		if err != nil || idx < 0 || idx >= len(batch) {
			return fmt.Errorf("hostus: result id %q does not map to a sent name", r.ID)
		}
		resolved[batch[idx]] = r.ConceptID
	}
	return nil
}

func (c *Client) resolveBatch(ctx context.Context, batch []string) ([]matchResult, error) {
	req := matchRequest{EntryBackbone: c.entryBackbone}
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
		// Transport failure (refused, reset, timed out): hostus is not
		// answering, which is an operational problem with hostus.
		return nil, fmt.Errorf("calling hostus /v1/match: %w: %w", output.ErrResolverUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit))
		// A 4xx says hostus understood the request and refused it — situs sent
		// something wrong (e.g. an unknown entry_backbone). Reporting that as
		// "hostus unavailable" would point an operator at the wrong system, so
		// only a 5xx counts as hostus being unavailable.
		if resp.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("hostus /v1/match: %w: status %s: %s",
				output.ErrResolverUnavailable, resp.Status, snippet)
		}
		return nil, fmt.Errorf("hostus /v1/match rejected the request: %w: status %s: %s",
			output.ErrResolverRejected, resp.Status, snippet)
	}

	var out matchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding hostus match response: %w", err)
	}
	return out.Results, nil
}
