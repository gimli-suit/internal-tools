package sync

import (
	"context"
	"errors"
	"log/slog"
	"testing"

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

func baseProjectData() *github.ProjectData {
	return &github.ProjectData{
		ProjectID:       "PVT_1",
		StatusFieldID:   "F1",
		ShippedOptionID: "O1",
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
		StatusUpdater:   updater,
		Org:             "tailscale",
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
		StatusUpdater:   updater,
		Org:             "tailscale",
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
		StatusUpdater:   updater,
		Org:             "tailscale",
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
		StatusUpdater:   updater,
		Org:             "tailscale",
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
		StatusUpdater:   updater,
		Org:             "tailscale",
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
		StatusUpdater:   updater,
		Org:             "tailscale",
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
		StatusUpdater:   &mockStatusUpdater{err: errors.New("permission denied")},
		Org:             "tailscale",
		Repo:            "corp",
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
		StatusUpdater:   updater,
		Org:             "tailscale",
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
