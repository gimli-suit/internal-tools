package sync

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/github-deployed-issue-sync/internal/github"
)

type mockProdver struct {
	sha string
	err error
}

func (m *mockProdver) FetchDeployedSHA(ctx context.Context) (string, error) {
	return m.sha, m.err
}

type mockProjectQuerier struct {
	data *github.ProjectData
	err  error
}

func (m *mockProjectQuerier) GetProjectItems(ctx context.Context) (*github.ProjectData, error) {
	return m.data, m.err
}

type mockAncestorChecker struct {
	results map[string]bool // keyed by "commitSHA"
	err     error
}

func (m *mockAncestorChecker) IsAncestor(ctx context.Context, owner, repo, commitSHA, deployedSHA string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.results[commitSHA], nil
}

type mockStatusUpdater struct {
	updated []string // item IDs
	err     error
}

func (m *mockStatusUpdater) UpdateItemStatus(ctx context.Context, projectID, itemID, fieldID, optionID string) error {
	if m.err != nil {
		return m.err
	}
	m.updated = append(m.updated, itemID)
	return nil
}

type mockIterationUpdater struct {
	updated map[string]string // item ID -> iteration ID
	err     error
}

func (m *mockIterationUpdater) UpdateItemIteration(ctx context.Context, projectID, itemID, fieldID, iterationID string) error {
	if m.err != nil {
		return m.err
	}
	if m.updated == nil {
		m.updated = make(map[string]string)
	}
	m.updated[itemID] = iterationID
	return nil
}

func baseProjectData() *github.ProjectData {
	return &github.ProjectData{
		ProjectID:        "PVT_1",
		StatusFieldID:    "F1",
		ShippedOptionID:  "O1",
		IterationFieldID: "IF1",
		Iterations: []github.Iteration{
			{ID: "iter_0", Title: "Sprint 0", StartDate: "2026-03-31", Duration: 14},
			{ID: "iter_1", Title: "Sprint 1", StartDate: "2026-04-14", Duration: 14},
		},
	}
}

func TestRun_HappyPath(t *testing.T) {
	pd := baseProjectData()
	pd.Items = []github.ProjectItem{
		{
			ItemID:        "I1",
			CurrentStatus: "In Progress",
			Issue: &github.Issue{
				Number:     10,
				Title:      "Fix bug",
				Closed:     true,
				Repository: "tailscale/corp",
				ClosingPRs: []github.PullRequest{
					{Number: 100, Merged: true, MergeCommit: "aaa", Repository: "tailscale/corp"},
				},
			},
		},
	}

	updater := &mockStatusUpdater{}
	s := &Syncer{
		Prodver:         &mockProdver{sha: "deployed123"},
		ProjectQuerier:  &mockProjectQuerier{data: pd},
		AncestorChecker: &mockAncestorChecker{results: map[string]bool{"aaa": true}},
		StatusUpdater:    updater,
		IterationUpdater: &mockIterationUpdater{},
		Org:              "tailscale",
		Repo:            "corp",
		Logger:          slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updater.updated) != 1 || updater.updated[0] != "I1" {
		t.Errorf("expected I1 updated, got %v", updater.updated)
	}
}

func TestRun_AlreadyShipped(t *testing.T) {
	pd := baseProjectData()
	pd.Items = []github.ProjectItem{
		{
			ItemID:        "I1",
			CurrentStatus: "🚢 Shipped",
			Issue: &github.Issue{
				Number:     10,
				Title:      "Already shipped",
				Closed:     true,
				Repository: "tailscale/corp",
				ClosingPRs: []github.PullRequest{
					{Number: 100, Merged: true, MergeCommit: "aaa", Repository: "tailscale/corp"},
				},
			},
		},
	}

	updater := &mockStatusUpdater{}
	s := &Syncer{
		Prodver:         &mockProdver{sha: "deployed123"},
		ProjectQuerier:  &mockProjectQuerier{data: pd},
		AncestorChecker: &mockAncestorChecker{results: map[string]bool{"aaa": true}},
		StatusUpdater:    updater,
		IterationUpdater: &mockIterationUpdater{},
		Org:              "tailscale",
		Repo:            "corp",
		Logger:          slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updater.updated) != 0 {
		t.Errorf("expected no updates, got %v", updater.updated)
	}
}

func TestRun_UnmergedPR(t *testing.T) {
	pd := baseProjectData()
	pd.Items = []github.ProjectItem{
		{
			ItemID:        "I1",
			CurrentStatus: "In Progress",
			Issue: &github.Issue{
				Number:     10,
				Title:      "Has unmerged PR",
				Closed:     true,
				Repository: "tailscale/corp",
				ClosingPRs: []github.PullRequest{
					{Number: 100, Merged: true, MergeCommit: "aaa", Repository: "tailscale/corp"},
					{Number: 101, Merged: false, Repository: "tailscale/corp"},
				},
			},
		},
	}

	updater := &mockStatusUpdater{}
	s := &Syncer{
		Prodver:         &mockProdver{sha: "deployed123"},
		ProjectQuerier:  &mockProjectQuerier{data: pd},
		AncestorChecker: &mockAncestorChecker{results: map[string]bool{"aaa": true}},
		StatusUpdater:    updater,
		IterationUpdater: &mockIterationUpdater{},
		Org:              "tailscale",
		Repo:            "corp",
		Logger:          slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updater.updated) != 0 {
		t.Errorf("expected no updates for unmerged PR, got %v", updater.updated)
	}
}

func TestRun_NoClosingPRs(t *testing.T) {
	pd := baseProjectData()
	pd.Items = []github.ProjectItem{
		{
			ItemID:        "I1",
			CurrentStatus: "In Progress",
			Issue: &github.Issue{
				Number:     10,
				Title:      "No PRs",
				Closed:     true,
				Repository: "tailscale/corp",
			},
		},
	}

	updater := &mockStatusUpdater{}
	s := &Syncer{
		Prodver:         &mockProdver{sha: "deployed123"},
		ProjectQuerier:  &mockProjectQuerier{data: pd},
		AncestorChecker: &mockAncestorChecker{},
		StatusUpdater:    updater,
		IterationUpdater: &mockIterationUpdater{},
		Org:              "tailscale",
		Repo:            "corp",
		Logger:          slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updater.updated) != 0 {
		t.Errorf("expected no updates for no PRs, got %v", updater.updated)
	}
}

func TestRun_NotYetDeployed(t *testing.T) {
	pd := baseProjectData()
	pd.Items = []github.ProjectItem{
		{
			ItemID:        "I1",
			CurrentStatus: "In Progress",
			Issue: &github.Issue{
				Number:     10,
				Title:      "Not deployed yet",
				Closed:     true,
				Repository: "tailscale/corp",
				ClosingPRs: []github.PullRequest{
					{Number: 100, Merged: true, MergeCommit: "aaa", Repository: "tailscale/corp"},
				},
			},
		},
	}

	updater := &mockStatusUpdater{}
	s := &Syncer{
		Prodver:         &mockProdver{sha: "deployed123"},
		ProjectQuerier:  &mockProjectQuerier{data: pd},
		AncestorChecker: &mockAncestorChecker{results: map[string]bool{"aaa": false}},
		StatusUpdater:    updater,
		IterationUpdater: &mockIterationUpdater{},
		Org:              "tailscale",
		Repo:            "corp",
		Logger:          slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updater.updated) != 0 {
		t.Errorf("expected no updates for undeployed PR, got %v", updater.updated)
	}
}

func TestRun_OnlyNonCorpPRs(t *testing.T) {
	pd := baseProjectData()
	pd.Items = []github.ProjectItem{
		{
			ItemID:        "I1",
			CurrentStatus: "In Progress",
			Issue: &github.Issue{
				Number:     10,
				Title:      "Only tailscale/tailscale PRs",
				Closed:     true,
				Repository: "tailscale/corp",
				ClosingPRs: []github.PullRequest{
					{Number: 100, Merged: true, MergeCommit: "aaa", Repository: "tailscale/tailscale"},
				},
			},
		},
	}

	updater := &mockStatusUpdater{}
	s := &Syncer{
		Prodver:         &mockProdver{sha: "deployed123"},
		ProjectQuerier:  &mockProjectQuerier{data: pd},
		AncestorChecker: &mockAncestorChecker{results: map[string]bool{"aaa": true}},
		StatusUpdater:    updater,
		IterationUpdater: &mockIterationUpdater{},
		Org:              "tailscale",
		Repo:            "corp",
		Logger:          slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updater.updated) != 0 {
		t.Errorf("expected no updates for non-corp PRs only, got %v", updater.updated)
	}
}

func TestRun_ProdverError(t *testing.T) {
	s := &Syncer{
		Prodver: &mockProdver{err: errors.New("network error")},
		Logger:  slog.Default(),
	}

	err := s.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from prodver failure")
	}
}

func TestRun_ProjectQueryError(t *testing.T) {
	s := &Syncer{
		Prodver:        &mockProdver{sha: "deployed123"},
		ProjectQuerier: &mockProjectQuerier{err: errors.New("graphql error")},
		Logger:         slog.Default(),
	}

	err := s.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from project query failure")
	}
}

func TestRun_UpdateError(t *testing.T) {
	pd := baseProjectData()
	pd.Items = []github.ProjectItem{
		{
			ItemID:        "I1",
			CurrentStatus: "In Progress",
			Issue: &github.Issue{
				Number:     10,
				Title:      "Fix bug",
				Closed:     true,
				Repository: "tailscale/corp",
				ClosingPRs: []github.PullRequest{
					{Number: 100, Merged: true, MergeCommit: "aaa", Repository: "tailscale/corp"},
				},
			},
		},
	}

	s := &Syncer{
		Prodver:         &mockProdver{sha: "deployed123"},
		ProjectQuerier:  &mockProjectQuerier{data: pd},
		AncestorChecker: &mockAncestorChecker{results: map[string]bool{"aaa": true}},
		StatusUpdater:    &mockStatusUpdater{err: errors.New("permission denied")},
		IterationUpdater: &mockIterationUpdater{},
		Org:              "tailscale",
		Repo:             "corp",
		Logger:          slog.Default(),
	}

	err := s.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from update failure")
	}
}

func TestRun_DraftIssueSkipped(t *testing.T) {
	pd := baseProjectData()
	pd.Items = []github.ProjectItem{
		{ItemID: "I1", CurrentStatus: "Todo", Issue: nil}, // DraftIssue
	}

	updater := &mockStatusUpdater{}
	s := &Syncer{
		Prodver:         &mockProdver{sha: "deployed123"},
		ProjectQuerier:  &mockProjectQuerier{data: pd},
		AncestorChecker: &mockAncestorChecker{},
		StatusUpdater:    updater,
		IterationUpdater: &mockIterationUpdater{},
		Org:              "tailscale",
		Repo:            "corp",
		Logger:          slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updater.updated) != 0 {
		t.Errorf("expected no updates for draft issue, got %v", updater.updated)
	}
}

func TestFindIteration(t *testing.T) {
	iterations := []github.Iteration{
		{ID: "iter_0", Title: "Sprint 0", StartDate: "2026-03-31", Duration: 14},
		{ID: "iter_1", Title: "Sprint 1", StartDate: "2026-04-14", Duration: 14},
	}

	tests := []struct {
		name     string
		closedAt string
		wantID   string
	}{
		{"start of Sprint 0", "2026-03-31T10:00:00Z", "iter_0"},
		{"middle of Sprint 0", "2026-04-05T10:00:00Z", "iter_0"},
		{"last day of Sprint 0", "2026-04-13T23:59:00Z", "iter_0"},
		{"start of Sprint 1", "2026-04-14T00:00:00Z", "iter_1"},
		{"middle of Sprint 1", "2026-04-20T15:00:00Z", "iter_1"},
		{"before all sprints", "2026-03-30T10:00:00Z", ""},
		{"after all sprints falls back to last", "2026-04-28T10:00:00Z", "iter_1"},
		{"gap between sprints falls back to previous", "2026-04-29T10:00:00Z", "iter_1"},
	}

	// Add a gap: Sprint 0 ends Apr 14, Sprint 1 starts Apr 14 — contiguous.
	// Use a different set with a gap to test fallback.
	gapIterations := []github.Iteration{
		{ID: "g0", Title: "Sprint A", StartDate: "2026-03-01", Duration: 14}, // ends Mar 15
		{ID: "g1", Title: "Sprint B", StartDate: "2026-04-01", Duration: 14}, // starts Apr 1
	}
	gapTests := []struct {
		name     string
		closedAt string
		wantID   string
	}{
		{"in gap falls back to previous", "2026-03-20T10:00:00Z", "g0"},
		{"in Sprint B", "2026-04-05T10:00:00Z", "g1"},
	}
	for _, tt := range gapTests {
		t.Run(tt.name, func(t *testing.T) {
			closedAt, _ := time.Parse(time.RFC3339, tt.closedAt)
			got := findIteration(gapIterations, closedAt)
			if got == nil {
				t.Fatalf("expected iteration %q, got nil", tt.wantID)
			}
			if got.ID != tt.wantID {
				t.Errorf("got iteration %q, want %q", got.ID, tt.wantID)
			}
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closedAt, _ := time.Parse(time.RFC3339, tt.closedAt)
			got := findIteration(iterations, closedAt)
			if tt.wantID == "" {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
			} else {
				if got == nil {
					t.Fatalf("expected iteration %q, got nil", tt.wantID)
				}
				if got.ID != tt.wantID {
					t.Errorf("got iteration %q, want %q", got.ID, tt.wantID)
				}
			}
		})
	}
}

func TestRun_IterationAssignment(t *testing.T) {
	pd := baseProjectData()
	pd.Items = []github.ProjectItem{
		{
			ItemID:        "I1",
			CurrentStatus: "🚀 Done",
			Issue: &github.Issue{
				Number:   10,
				Title:    "Done issue",
				Closed:   true,
				ClosedAt: "2026-04-15T10:00:00Z",
			},
		},
		{
			ItemID:             "I2",
			CurrentStatus:      "🚢 Shipped",
			CurrentIterationID: "iter_1",
			Issue: &github.Issue{
				Number:   11,
				Title:    "Already has iteration",
				Closed:   true,
				ClosedAt: "2026-04-15T10:00:00Z",
			},
		},
		{
			ItemID:        "I3",
			CurrentStatus: "In Progress",
			Issue: &github.Issue{
				Number: 12,
				Title:  "Not done yet",
				Closed: false,
			},
		},
	}

	iterUpdater := &mockIterationUpdater{}
	s := &Syncer{
		Prodver:          &mockProdver{sha: "deployed123"},
		ProjectQuerier:   &mockProjectQuerier{data: pd},
		AncestorChecker:  &mockAncestorChecker{},
		StatusUpdater:    &mockStatusUpdater{},
		IterationUpdater: iterUpdater,
		Org:              "tailscale",
		Repo:             "corp",
		Logger:           slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// I1 should get iteration set (Done, no iteration, closedAt matches Sprint 1)
	if iterUpdater.updated["I1"] != "iter_1" {
		t.Errorf("I1 iteration = %q, want iter_1", iterUpdater.updated["I1"])
	}
	// I2 already has iteration, should not be updated
	if _, ok := iterUpdater.updated["I2"]; ok {
		t.Error("I2 should not have iteration updated (already set)")
	}
	// I3 is not Done/Shipped, should not be updated
	if _, ok := iterUpdater.updated["I3"]; ok {
		t.Error("I3 should not have iteration updated (not Done/Shipped)")
	}
}
