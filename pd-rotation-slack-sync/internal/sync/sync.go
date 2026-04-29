package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

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
		name := m.DisplayName()
		s.Logger.Info("syncing mapping", "team", name, "usergroup_id", m.SlackUserGroupID)

		if err := s.syncOne(ctx, m); err != nil {
			s.Logger.Error("mapping failed", "team", name, "error", err)
			errs = append(errs, fmt.Errorf("%s -> usergroup %s: %w", name, m.SlackUserGroupID, err))
		}
	}

	return errors.Join(errs...)
}

func (s *Syncer) syncOne(ctx context.Context, m config.Mapping) error {
	name := m.DisplayName()
	email, err := s.PD.GetCurrentOnCallEmail(ctx, m.PagerDutyScheduleID)
	if err != nil {
		return fmt.Errorf("pagerduty: get on-call: %w", err)
	}
	s.Logger.Info("found on-call user", "team", name, "email", email)

	slackUserID, err := s.Slack.LookupUserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("slack: lookup user by email %q: %w", email, err)
	}
	s.Logger.Info("resolved slack user", "email", email, "user_id", slackUserID)

	// Check current members to detect changes.
	currentMembers, err := s.Slack.GetUserGroupMembers(ctx, m.SlackUserGroupID)
	if err != nil {
		s.Logger.Warn("could not fetch current group members, skipping DM", "error", err)
		currentMembers = nil
	}

	if err := s.Slack.UpdateUserGroupMembers(ctx, m.SlackUserGroupID, []string{slackUserID}); err != nil {
		return fmt.Errorf("slack: update usergroup: %w", err)
	}
	s.Logger.Info("sync complete", "team", name, "user_id", slackUserID)

	// Notify if the on-call user changed.
	if !containsUser(currentMembers, slackUserID) {
		// DM the new on-call user.
		msg := "You have been added to the on-call user group."
		if err := s.Slack.PostMessage(ctx, slackUserID, msg); err != nil {
			s.Logger.Warn("failed to DM on-call user", "user_id", slackUserID, "error", err)
		} else {
			s.Logger.Info("notified on-call user via DM", "user_id", slackUserID)
		}

		// Post to the team channel if configured.
		if m.SlackChannelID != "" && m.NotificationMessage != "" {
			channelMsg := strings.ReplaceAll(m.NotificationMessage, "{@user}", fmt.Sprintf("<@%s>", slackUserID))
			if err := s.Slack.PostMessage(ctx, m.SlackChannelID, channelMsg); err != nil {
				s.Logger.Warn("failed to post channel notification", "channel", m.SlackChannelID, "error", err)
			} else {
				s.Logger.Info("posted channel notification", "channel", m.SlackChannelID)
			}
		}
	}

	return nil
}

func containsUser(members []string, userID string) bool {
	for _, m := range members {
		if m == userID {
			return true
		}
	}
	return false
}
