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

func makeGraphQLResponse(data any) string {
	d, _ := json.Marshal(data)
	return `{"data":` + string(d) + `}`
}

func TestGetProjectItems(t *testing.T) {
	response := makeGraphQLResponse(projectQueryResponse{
		Organization: struct {
			ProjectV2 struct {
				ID    string `json:"id"`
				Field struct {
					ID      string `json:"id"`
					Options []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"options"`
				} `json:"field"`
				Items struct {
					TotalCount int `json:"totalCount"`
					PageInfo   struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []projectItemNode `json:"nodes"`
				} `json:"items"`
			} `json:"projectV2"`
		}{
			ProjectV2: struct {
				ID    string `json:"id"`
				Field struct {
					ID      string `json:"id"`
					Options []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"options"`
				} `json:"field"`
				Items struct {
					TotalCount int `json:"totalCount"`
					PageInfo   struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []projectItemNode `json:"nodes"`
				} `json:"items"`
			}{
				ID: "PVT_123",
				Field: struct {
					ID      string `json:"id"`
					Options []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"options"`
				}{
					ID: "PVTSSF_status",
					Options: []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					}{
						{ID: "opt_todo", Name: "Todo"},
						{ID: "opt_shipped", Name: "\U0001F6A2 Shipped"},
					},
				},
				Items: struct {
					TotalCount int `json:"totalCount"`
					PageInfo   struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []projectItemNode `json:"nodes"`
				}{
					TotalCount: 1,
					Nodes: []projectItemNode{
						{
							ID: "PVTI_item1",
							FieldValueByName: struct {
								Name string `json:"name"`
							}{Name: "Todo"},
							Content: struct {
								Number     int    `json:"number"`
								Title      string `json:"title"`
								State      string `json:"state"`
								Repository *struct {
									NameWithOwner string `json:"nameWithOwner"`
								} `json:"repository"`
								TimelineItems *struct {
									Nodes []timelineEventNode `json:"nodes"`
								} `json:"timelineItems"`
							}{
								Number: 42,
								Title:  "Fix the thing",
								State:  "CLOSED",
								Repository: &struct {
									NameWithOwner string `json:"nameWithOwner"`
								}{NameWithOwner: "tailscale/corp"},
								TimelineItems: &struct {
									Nodes []timelineEventNode `json:"nodes"`
								}{
									Nodes: []timelineEventNode{
										{
											Typename:        "CrossReferencedEvent",
											WillCloseTarget: true,
											Source: struct {
												Number      int  `json:"number"`
												Merged      bool `json:"merged"`
												MergeCommit *struct {
													OID string `json:"oid"`
												} `json:"mergeCommit"`
												Repository *struct {
													NameWithOwner string `json:"nameWithOwner"`
												} `json:"repository"`
											}{
												Number: 100,
												Merged: true,
												MergeCommit: &struct {
													OID string `json:"oid"`
												}{OID: "abc123"},
												Repository: &struct {
													NameWithOwner string `json:"nameWithOwner"`
												}{NameWithOwner: "tailscale/corp"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})

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
			resp := `{"data":{"organization":{"projectV2":{"id":"PVT_1","field":{"id":"F1","options":[{"id":"O1","name":"🚢 Shipped"}]},"items":{"pageInfo":{"hasNextPage":true,"endCursor":"cursor1"},"nodes":[{"id":"I1","fieldValueByName":{"name":"Todo"},"content":{"number":1,"title":"Issue 1","repository":{"nameWithOwner":"tailscale/corp"},"timelineItems":{"nodes":[]}}}]}}}}}`
			w.Write([]byte(resp))
		} else {
			// Verify cursor was passed.
			if !strings.Contains(string(body), "cursor1") {
				t.Error("expected cursor1 in second request")
			}
			resp := `{"data":{"organization":{"projectV2":{"id":"PVT_1","field":{"id":"F1","options":[{"id":"O1","name":"🚢 Shipped"}]},"items":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"id":"I2","fieldValueByName":{"name":"Todo"},"content":{"number":2,"title":"Issue 2","repository":{"nameWithOwner":"tailscale/corp"},"timelineItems":{"nodes":[]}}}]}}}}}`
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
