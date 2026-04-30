package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/github-deployed-issue-sync/internal/github"
	"github.com/github-deployed-issue-sync/internal/prodver"
)

// Syncer orchestrates the deployed-issue sync process.
type Syncer struct {
	Prodver          prodver.SHAFetcher
	ProjectQuerier   github.ProjectQuerier
	AncestorChecker  github.AncestorChecker
	StatusUpdater    github.StatusUpdater
	IterationUpdater github.IterationUpdater
	Org              string
	Repo             string
	DryRun           bool
	Logger           *slog.Logger
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
		return fmt.Errorf("no '🚢 Shipped' status option found in project; available options: %v", projectData.StatusOptions)
	}

	targetRepo := s.Org + "/" + s.Repo
	var updated, skipped int
	var errs []error

	var noIssue, alreadyShipped, notClosed, noPRs, unmerged, noCorpPRs, notDeployed int

	for _, item := range projectData.Items {
		if item.Issue == nil {
			noIssue++
			skipped++
			continue
		}

		if strings.Join(strings.Fields(item.CurrentStatus), " ") == "🚢 Shipped" {
			alreadyShipped++
			skipped++
			continue
		}

		if !item.Issue.Closed {
			notClosed++
			skipped++
			continue
		}

		if len(item.Issue.ClosingPRs) == 0 {
			noPRs++
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
			unmerged++
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
			noCorpPRs++
			skipped++
			continue
		}

		if !allDeployed {
			notDeployed++
			s.Logger.Debug("not yet deployed", "issue", item.Issue.Title, "number", item.Issue.Number)
			skipped++
			continue
		}

		issueURL := fmt.Sprintf("https://github.com/%s/%s/issues/%d", s.Org, s.Repo, item.Issue.Number)

		if s.DryRun {
			s.Logger.Info("would mark as shipped (dry-run)", "issue", item.Issue.Title, "number", item.Issue.Number, "url", issueURL)
			updated++
			continue
		}

		if err := s.StatusUpdater.UpdateItemStatus(ctx, projectData.ProjectID, item.ItemID, projectData.StatusFieldID, projectData.ShippedOptionID); err != nil {
			errs = append(errs, fmt.Errorf("issue #%d: update status: %w", item.Issue.Number, err))
			continue
		}

		updated++
		s.Logger.Info("marked as shipped", "issue", item.Issue.Title, "number", item.Issue.Number, "url", issueURL)
	}

	s.Logger.Info("shipping sync complete",
		"updated", updated,
		"skipped", skipped,
		"skip_no_issue", noIssue,
		"skip_already_shipped", alreadyShipped,
		"skip_not_closed", notClosed,
		"skip_no_closing_prs", noPRs,
		"skip_unmerged", unmerged,
		"skip_no_corp_prs", noCorpPRs,
		"skip_not_deployed", notDeployed,
		"errors", len(errs),
	)

	// Second pass: assign iterations to Done/Shipped items missing one.
	iterErrs := s.assignIterations(ctx, projectData)
	errs = append(errs, iterErrs...)

	return errors.Join(errs...)
}

// assignIterations sets the iteration field for Done/Shipped items that don't have one,
// based on the issue's closedAt date.
func (s *Syncer) assignIterations(ctx context.Context, pd *github.ProjectData) []error {
	if pd.IterationFieldID == "" {
		s.Logger.Warn("no iteration field found in project, skipping iteration assignment")
		return nil
	}
	if len(pd.Iterations) == 0 {
		s.Logger.Warn("no iterations configured in project, skipping iteration assignment")
		return nil
	}

	var iterUpdated, iterSkipped int
	var noIssueCnt, notDoneShipped, hasIteration, noClosedAt, noMatchingIter int
	var errs []error

	for _, item := range pd.Items {
		if item.Issue == nil {
			noIssueCnt++
			iterSkipped++
			continue
		}

		status := strings.Join(strings.Fields(item.CurrentStatus), " ")
		if status != "🚀 Done" && status != "🚢 Shipped" {
			notDoneShipped++
			iterSkipped++
			continue
		}

		if item.CurrentIterationID != "" {
			hasIteration++
			iterSkipped++
			continue
		}

		if item.Issue.ClosedAt == "" {
			noClosedAt++
			iterSkipped++
			continue
		}

		closedAt, err := time.Parse(time.RFC3339, item.Issue.ClosedAt)
		if err != nil {
			errs = append(errs, fmt.Errorf("issue #%d: parsing closedAt %q: %w", item.Issue.Number, item.Issue.ClosedAt, err))
			continue
		}

		iter := findIteration(pd.Iterations, closedAt)
		if iter == nil {
			noMatchingIter++
			iterSkipped++
			continue
		}

		if s.DryRun {
			s.Logger.Info("would set iteration (dry-run)",
				"issue", item.Issue.Title, "number", item.Issue.Number,
				"iteration", iter.Title, "closedAt", item.Issue.ClosedAt)
			iterUpdated++
			continue
		}

		if err := s.IterationUpdater.UpdateItemIteration(ctx, pd.ProjectID, item.ItemID, pd.IterationFieldID, iter.ID); err != nil {
			errs = append(errs, fmt.Errorf("issue #%d: set iteration: %w", item.Issue.Number, err))
			continue
		}

		iterUpdated++
		s.Logger.Info("set iteration", "issue", item.Issue.Title, "number", item.Issue.Number, "iteration", iter.Title)
	}

	s.Logger.Info("iteration sync complete",
		"updated", iterUpdated,
		"skipped", iterSkipped,
		"skip_no_issue", noIssueCnt,
		"skip_not_done_or_shipped", notDoneShipped,
		"skip_has_iteration", hasIteration,
		"skip_no_closed_at", noClosedAt,
		"skip_no_matching_iteration", noMatchingIter,
		"errors", len(errs),
	)

	return errs
}

// findIteration returns the iteration whose date range contains the given time.
// If the date falls in a gap between iterations, it returns the most recently
// ended iteration before the close date.
func findIteration(iterations []github.Iteration, closedAt time.Time) *github.Iteration {
	closedDate := closedAt.UTC().Truncate(24 * time.Hour)

	// First pass: exact match.
	for i := range iterations {
		start, err := time.Parse("2006-01-02", iterations[i].StartDate)
		if err != nil {
			continue
		}
		end := start.AddDate(0, 0, iterations[i].Duration)
		if !closedDate.Before(start) && closedDate.Before(end) {
			return &iterations[i]
		}
	}

	// Second pass: fall back to the most recently ended iteration before closedDate.
	var best *github.Iteration
	var bestEnd time.Time
	for i := range iterations {
		start, err := time.Parse("2006-01-02", iterations[i].StartDate)
		if err != nil {
			continue
		}
		end := start.AddDate(0, 0, iterations[i].Duration)
		if !end.After(closedDate) && (best == nil || end.After(bestEnd)) {
			best = &iterations[i]
			bestEnd = end
		}
	}
	return best
}
