package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetProjectItems(t *testing.T) {
	response := `{"data":{"organization":{"projectV2":{
		"id":"PVT_123",
		"statusField":{"id":"PVTSSF_status","options":[{"id":"opt_todo","name":"Todo"},{"id":"opt_shipped","name":"🚢 Shipped"}]},
		"iterationField":{"id":"PVTIF_iter","configuration":{
			"iterations":[{"id":"iter_1","title":"Sprint 1","startDate":"2026-04-14","duration":14}],
			"completedIterations":[{"id":"iter_0","title":"Sprint 0","startDate":"2026-03-31","duration":14}]
		}},
		"items":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{
			"id":"PVTI_item1",
			"statusValue":{"name":"Todo"},
			"iterationValue":{"iterationId":""},
			"content":{
				"number":42,"title":"Fix the thing","state":"CLOSED","closedAt":"2026-04-15T10:30:00Z",
				"repository":{"nameWithOwner":"tailscale/corp"},
				"timelineItems":{"nodes":[{
					"__typename":"CrossReferencedEvent",
					"willCloseTarget":true,
					"source":{"number":100,"merged":true,"mergeCommit":{"oid":"abc123"},"repository":{"nameWithOwner":"tailscale/corp"}}
				}]}
			}
		}]}
	}}}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or wrong auth header")
		}
		w.Write([]byte(response))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient:    srv.Client(),
		Token:         "test-token",
		GraphQLURL:    srv.URL,
		Org:           "tailscale",
		ProjectNumber: 1,
	}

	pd, err := c.GetProjectItems(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pd.ProjectID != "PVT_123" {
		t.Errorf("got project ID %q, want %q", pd.ProjectID, "PVT_123")
	}
	if pd.StatusFieldID != "PVTSSF_status" {
		t.Errorf("got field ID %q, want %q", pd.StatusFieldID, "PVTSSF_status")
	}
	if pd.ShippedOptionID != "opt_shipped" {
		t.Errorf("got shipped option ID %q, want %q", pd.ShippedOptionID, "opt_shipped")
	}
	if len(pd.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(pd.Items))
	}
	item := pd.Items[0]
	if item.Issue == nil {
		t.Fatal("expected issue, got nil")
	}
	if item.Issue.Number != 42 {
		t.Errorf("got issue #%d, want #42", item.Issue.Number)
	}
	if len(item.Issue.ClosingPRs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(item.Issue.ClosingPRs))
	}
	pr := item.Issue.ClosingPRs[0]
	if !pr.Merged || pr.MergeCommit != "abc123" {
		t.Errorf("got PR merged=%v commit=%q, want merged=true commit=abc123", pr.Merged, pr.MergeCommit)
	}
	if pd.IterationFieldID != "PVTIF_iter" {
		t.Errorf("got iteration field ID %q, want %q", pd.IterationFieldID, "PVTIF_iter")
	}
	if len(pd.Iterations) != 2 {
		t.Fatalf("got %d iterations, want 2", len(pd.Iterations))
	}
	if pd.Iterations[0].Title != "Sprint 1" {
		t.Errorf("got iteration 0 title %q, want %q", pd.Iterations[0].Title, "Sprint 1")
	}
	if !item.Issue.Closed || item.Issue.ClosedAt != "2026-04-15T10:30:00Z" {
		t.Errorf("got closed=%v closedAt=%q", item.Issue.Closed, item.Issue.ClosedAt)
	}
}

func TestGetProjectItems_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errors":[{"message":"Not found"}]}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient:    srv.Client(),
		Token:         "test-token",
		GraphQLURL:    srv.URL,
		Org:           "tailscale",
		ProjectNumber: 1,
	}

	_, err := c.GetProjectItems(context.Background())
	if err == nil {
		t.Fatal("expected error for GraphQL error response")
	}
}

func TestGetProjectItems_Pagination(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		page++

		if page == 1 {
			// First page — has next page.
			resp := `{"data":{"organization":{"projectV2":{"id":"PVT_1","statusField":{"id":"F1","options":[{"id":"O1","name":"🚢 Shipped"}]},"iterationField":{"id":"IF1","configuration":{"iterations":[],"completedIterations":[]}},"items":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"cursor1"},"nodes":[{"id":"I1","statusValue":{"name":"Todo"},"iterationValue":{},"content":{"number":1,"title":"Issue 1","state":"OPEN","repository":{"nameWithOwner":"tailscale/corp"},"timelineItems":{"nodes":[]}}}]}}}}}`
			w.Write([]byte(resp))
		} else {
			// Verify cursor was passed.
			if !strings.Contains(string(body), "cursor1") {
				t.Error("expected cursor1 in second request")
			}
			resp := `{"data":{"organization":{"projectV2":{"id":"PVT_1","statusField":{"id":"F1","options":[{"id":"O1","name":"🚢 Shipped"}]},"iterationField":{"id":"IF1","configuration":{"iterations":[],"completedIterations":[]}},"items":{"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"id":"I2","statusValue":{"name":"Todo"},"iterationValue":{},"content":{"number":2,"title":"Issue 2","state":"OPEN","repository":{"nameWithOwner":"tailscale/corp"},"timelineItems":{"nodes":[]}}}]}}}}}`
			w.Write([]byte(resp))
		}
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient:    srv.Client(),
		Token:         "test-token",
		GraphQLURL:    srv.URL,
		Org:           "tailscale",
		ProjectNumber: 1,
	}

	pd, err := c.GetProjectItems(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pd.Items) != 2 {
		t.Errorf("got %d items, want 2", len(pd.Items))
	}
	if page != 2 {
		t.Errorf("expected 2 pages fetched, got %d", page)
	}
}

func TestIsAncestor_Behind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/tailscale/corp/compare/def...abc" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"status":"behind"}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient:  srv.Client(),
		Token:       "test-token",
		RestBaseURL: srv.URL,
	}

	ok, err := c.IsAncestor(context.Background(), "tailscale", "corp", "abc", "def")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true for 'behind' status")
	}
}

func TestIsAncestor_Identical(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"identical"}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient:  srv.Client(),
		Token:       "test-token",
		RestBaseURL: srv.URL,
	}

	ok, err := c.IsAncestor(context.Background(), "tailscale", "corp", "abc", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected true for 'identical' status")
	}
}

func TestIsAncestor_Ahead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ahead"}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient:  srv.Client(),
		Token:       "test-token",
		RestBaseURL: srv.URL,
	}

	ok, err := c.IsAncestor(context.Background(), "tailscale", "corp", "abc", "def")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for 'ahead' status")
	}
}

func TestIsAncestor_Diverged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"diverged"}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient:  srv.Client(),
		Token:       "test-token",
		RestBaseURL: srv.URL,
	}

	ok, err := c.IsAncestor(context.Background(), "tailscale", "corp", "abc", "def")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for 'diverged' status")
	}
}

func TestIsAncestor_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient:  srv.Client(),
		Token:       "test-token",
		RestBaseURL: srv.URL,
	}

	ok, err := c.IsAncestor(context.Background(), "tailscale", "corp", "abc", "def")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for 404")
	}
}

func TestUpdateItemStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		json.Unmarshal(body, &req)

		if req.Variables["projectId"] != "PVT_1" {
			t.Errorf("got projectId %v", req.Variables["projectId"])
		}
		if req.Variables["itemId"] != "PVTI_1" {
			t.Errorf("got itemId %v", req.Variables["itemId"])
		}

		w.Write([]byte(`{"data":{"updateProjectV2ItemFieldValue":{"projectV2Item":{"id":"PVTI_1"}}}}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		Token:      "test-token",
		GraphQLURL: srv.URL,
	}

	err := c.UpdateItemStatus(context.Background(), "PVT_1", "PVTI_1", "F1", "O1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateItemStatus_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errors":[{"message":"Insufficient permissions"}]}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		Token:      "test-token",
		GraphQLURL: srv.URL,
	}

	err := c.UpdateItemStatus(context.Background(), "PVT_1", "PVTI_1", "F1", "O1")
	if err == nil {
		t.Fatal("expected error for GraphQL error response")
	}
}
