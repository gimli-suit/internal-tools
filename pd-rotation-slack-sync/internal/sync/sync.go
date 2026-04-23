package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/pd-rotation-slack-sync/internal/config"
	"github.com/pd-rotation-slack-sync/internal/pagerduty"
	"github.com/pd-rotation-slack-sync/internal/slack"
)

type Syncer struct {
	PD       pagerduty.OnCallGetter
	Slack    slack.UserGroupUpdater
	Mappings []config.Mapping
	Logger   *slog.Logger
}

func (s *Syncer) Run(ctx context.Context) error {
	var errs []error

	for _, m := range s.Mappings {
		s.Logger.Info("syncing mapping", "schedule_id", m.PagerDutyScheduleID, "usergroup_id", m.SlackUserGroupID)

		if err := s.syncOne(ctx, m); err != nil {
			s.Logger.Error("mapping failed", "schedule_id", m.PagerDutyScheduleID, "usergroup_id", m.SlackUserGroupID, "error", err)
			errs = append(errs, fmt.Errorf("schedule %s -> usergroup %s: %w", m.PagerDutyScheduleID, m.SlackUserGroupID, err))
		}
	}

	return errors.Join(errs...)
}

func (s *Syncer) syncOne(ctx context.Context, m config.Mapping) error {
	email, err := s.PD.GetCurrentOnCallEmail(ctx, m.PagerDutyScheduleID)
	if err != nil {
		return fmt.Errorf("pagerduty: get on-call: %w", err)
	}
	s.Logger.Info("found on-call user", "schedule_id", m.PagerDutyScheduleID, "email", email)

	slackUserID, err := s.Slack.LookupUserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("slack: lookup user by email %q: %w", email, err)
	}
	s.Logger.Info("resolved slack user", "email", email, "user_id", slackUserID)

	if err := s.Slack.UpdateUserGroupMembers(ctx, m.SlackUserGroupID, []string{slackUserID}); err != nil {
		return fmt.Errorf("slack: update usergroup: %w", err)
	}
	s.Logger.Info("sync complete", "schedule_id", m.PagerDutyScheduleID, "usergroup_id", m.SlackUserGroupID, "user_id", slackUserID)

	return nil
}
