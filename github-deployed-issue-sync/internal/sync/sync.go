package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/github-deployed-issue-sync/internal/github"
	"github.com/github-deployed-issue-sync/internal/prodver"
)

// Syncer orchestrates the deployed-issue sync process.
type Syncer struct {
	Prodver         prodver.SHAFetcher
	ProjectQuerier  github.ProjectQuerier
	AncestorChecker github.AncestorChecker
	StatusUpdater   github.StatusUpdater
	Org             string
	Repo            string
	Logger          *slog.Logger
}

// Run performs the sync: fetch deployed SHA, check project issues, update shipped status.
func (s *Syncer) Run(ctx context.Context) error {
	deployedSHA, err := s.Prodver.FetchDeployedSHA(ctx)
	if err != nil {
		return fmt.Errorf("fetching deployed SHA: %w", err)
	}
	s.Logger.Info("fetched deployed SHA", "sha", deployedSHA)

	projectData, err := s.ProjectQuerier.GetProjectItems(ctx)
	if err != nil {
		return fmt.Errorf("fetching project items: %w", err)
	}
	s.Logger.Info("fetched project items", "count", len(projectData.Items))

	if projectData.ShippedOptionID == "" {
		return fmt.Errorf("no '🚢 Shipped' status option found in project")
	}

	targetRepo := s.Org + "/" + s.Repo
	var updated, skipped int
	var errs []error

	for _, item := range projectData.Items {
		if item.Issue == nil {
			skipped++
			continue
		}

		if item.CurrentStatus == "🚢 Shipped" {
			skipped++
			continue
		}

		if len(item.Issue.ClosingPRs) == 0 {
			skipped++
			continue
		}

		// Check all PRs are merged.
		allMerged := true
		for _, pr := range item.Issue.ClosingPRs {
			if !pr.Merged {
				allMerged = false
				break
			}
		}
		if !allMerged {
			skipped++
			continue
		}

		// Check ancestor status for merged PRs in the target repo.
		var corpPRCount int
		allDeployed := true
		for _, pr := range item.Issue.ClosingPRs {
			if pr.Repository != targetRepo {
				continue
			}
			corpPRCount++

			isAnc, err := s.AncestorChecker.IsAncestor(ctx, s.Org, s.Repo, pr.MergeCommit, deployedSHA)
			if err != nil {
				errs = append(errs, fmt.Errorf("issue #%d: ancestor check for PR #%d: %w", item.Issue.Number, pr.Number, err))
				allDeployed = false
				break
			}
			if !isAnc {
				allDeployed = false
				break
			}
		}

		// Skip if no PRs in the target repo (don't mark shipped vacuously).
		if corpPRCount == 0 {
			skipped++
			continue
		}

		if !allDeployed {
			skipped++
			continue
		}

		if err := s.StatusUpdater.UpdateItemStatus(ctx, projectData.ProjectID, item.ItemID, projectData.StatusFieldID, projectData.ShippedOptionID); err != nil {
			errs = append(errs, fmt.Errorf("issue #%d: update status: %w", item.Issue.Number, err))
			continue
		}

		updated++
		s.Logger.Info("marked as shipped", "issue", item.Issue.Title, "number", item.Issue.Number)
	}

	s.Logger.Info("sync complete", "updated", updated, "skipped", skipped, "errors", len(errs))
	return errors.Join(errs...)
}
