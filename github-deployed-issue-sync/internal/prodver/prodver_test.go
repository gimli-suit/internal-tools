package prodver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleHTML = `<html><body>
<table cellpadding=5 border=1><tr>
<th>Server</th><th>Version</th><th>Process Start</th><th>Connected</th><th>Binary</th>
</tr>
<tr><td class=name><a href='http://control.corp.ts.net:8383/debug/'>control</a></td><td class=ver>1.97.255-t<a href='https://github.com/tailscale/tailscale/commits/12813dee0'>12813dee0</a>-g<a href='https://github.com/tailscale/corp/commits/6337698d2'>6337698d2</a>; sm=109fbf7</td><td>2026-04-22T15:20:32Z</td><td>23h58m39s</td><td>tailcontrol</td></tr>
<tr><td class=name><a href='http://shard1.corp.ts.net:8383/debug/'>shard1</a></td><td class=ver>1.97.255-t<a href='https://github.com/tailscale/tailscale/commits/aaa111bbb'>aaa111bbb</a>-g<a href='https://github.com/tailscale/corp/commits/abc123def'>abc123def</a>; sm=109fbf7</td><td>2026-04-22T15:17:25Z</td><td>24h1m47s</td><td>tailcontrol</td></tr>
<tr><td class=name><a href='http://shard10.corp.ts.net:8383/debug/'>shard10</a></td><td class=ver>1.97.255-t<a href='https://github.com/tailscale/tailscale/commits/ccc333ddd'>ccc333ddd</a>-g<a href='https://github.com/tailscale/corp/commits/def456ghi'>def456ghi</a>; sm=109fbf7</td><td>2026-04-22T14:29:24Z</td><td>24h49m47s</td><td>tailcontrol</td></tr>
</table></body></html>`

func TestFetchDeployedSHA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleHTML))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		ShardName:  "shard1",
	}

	sha, err := c.FetchDeployedSHA(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sha != "abc123def" {
		t.Errorf("got SHA %q, want %q", sha, "abc123def")
	}
}

func TestFetchDeployedSHA_NoPartialMatch(t *testing.T) {
	// "shard1" should not match "shard10"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleHTML))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		ShardName:  "shard10",
	}

	sha, err := c.FetchDeployedSHA(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sha != "def456ghi" {
		t.Errorf("got SHA %q, want %q", sha, "def456ghi")
	}
}

func TestFetchDeployedSHA_ShardNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleHTML))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		ShardName:  "shard99",
	}

	_, err := c.FetchDeployedSHA(context.Background())
	if err == nil {
		t.Fatal("expected error for missing shard")
	}
}

func TestFetchDeployedSHA_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		URL:        srv.URL,
		ShardName:  "shard1",
	}

	_, err := c.FetchDeployedSHA(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestParseSHA_NoCorphref(t *testing.T) {
	html := `<tr><td class=name><a href='x'>shard1</a></td><td>some version without corp link</td></tr>`
	_, err := parseSHA(html, "shard1")
	if err == nil {
		t.Fatal("expected error for missing corp commit link")
	}
}
