package prodver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SHAFetcher fetches the deployed SHA for a shard.
type SHAFetcher interface {
	FetchDeployedSHA(ctx context.Context) (string, error)
}

// Client fetches and parses the prodver HTML page.
type Client struct {
	HTTPClient *http.Client
	URL        string
	ShardName  string
}

// FetchDeployedSHA fetches the prodver page and extracts the tailscale/corp
// commit SHA for the configured shard.
func (c *Client) FetchDeployedSHA(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching prodver: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("prodver returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	return parseSHA(string(body), c.ShardName)
}

const corpCommitsPrefix = "tailscale/corp/commits/"

// parseSHA finds the row for the given shard and extracts the corp SHA.
func parseSHA(html, shardName string) (string, error) {
	rows := strings.Split(html, "<tr")
	for _, row := range rows {
		// Check if this row contains the shard name.
		// The shard name appears in a <td> like: <td class=name>...<a ...>shard1</a></td>
		// Match on ">shardName</a>" to avoid partial matches (e.g., "shard1" matching "shard10").
		marker := ">" + shardName + "</a>"
		if !strings.Contains(row, marker) {
			continue
		}

		// Find the corp SHA in this row via the anchor href.
		idx := strings.Index(row, corpCommitsPrefix)
		if idx == -1 {
			return "", fmt.Errorf("shard %q found but no corp commit link in row", shardName)
		}

		// Extract SHA from href: ...corp/commits/6337698d2'>6337698d2</a>
		start := idx + len(corpCommitsPrefix)
		rest := row[start:]
		// SHA ends at the next quote (') or (")
		end := strings.IndexAny(rest, "'\"")
		if end == -1 {
			return "", fmt.Errorf("shard %q: could not parse SHA from href", shardName)
		}

		sha := rest[:end]
		if sha == "" {
			return "", fmt.Errorf("shard %q: empty SHA in href", shardName)
		}
		return sha, nil
	}

	return "", fmt.Errorf("shard %q not found in prodver output", shardName)
}
