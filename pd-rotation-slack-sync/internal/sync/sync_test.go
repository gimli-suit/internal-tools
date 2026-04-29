package sync

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/pd-rotation-slack-sync/internal/config"
)

type mockPD struct {
	emails map[string]string // schedule ID -> email
	err    error
}

func (m *mockPD) GetCurrentOnCallEmail(_ context.Context, scheduleID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	email, ok := m.emails[scheduleID]
	if !ok {
		return "", errors.New("schedule not found")
	}
	return email, nil
}

type mockSlack struct {
	users     map[string]string // email -> user ID
	lookupErr error
	updateErr error
	msgErr    error

	groupMembers  map[string][]string      // group ID -> current member IDs
	membersErr    error
	updatedGroups map[string][]string      // group ID -> user IDs
	messages      map[string][]string      // channel/userID -> list of messages sent
}

func newMockSlack() *mockSlack {
	return &mockSlack{
		users:         make(map[string]string),
		groupMembers:  make(map[string][]string),
		updatedGroups: make(map[string][]string),
		messages:      make(map[string][]string),
	}
}

func (m *mockSlack) LookupUserByEmail(_ context.Context, email string) (string, error) {
	if m.lookupErr != nil {
		return "", m.lookupErr
	}
	id, ok := m.users[email]
	if !ok {
		return "", errors.New("user not found")
	}
	return id, nil
}

func (m *mockSlack) GetUserGroupMembers(_ context.Context, groupID string) ([]string, error) {
	if m.membersErr != nil {
		return nil, m.membersErr
	}
	return m.groupMembers[groupID], nil
}

func (m *mockSlack) UpdateUserGroupMembers(_ context.Context, groupID string, userIDs []string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updatedGroups[groupID] = userIDs
	return nil
}

func (m *mockSlack) PostMessage(_ context.Context, channelID, text string) error {
	if m.msgErr != nil {
		return m.msgErr
	}
	m.messages[channelID] = append(m.messages[channelID], text)
	return nil
}

func TestRun_SingleMapping(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"PSCHED1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"

	s := &Syncer{
		PD:    pd,
		Slack: sl,
		Mappings: []config.Mapping{
			{PagerDutyScheduleID: "PSCHED1", SlackUserGroupID: "S456"},
		},
		Logger: slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	users, ok := sl.updatedGroups["S456"]
	if !ok || len(users) != 1 || users[0] != "U123" {
		t.Errorf("updated groups = %v, want S456->[U123]", sl.updatedGroups)
	}
}

func TestRun_MultipleMappings(t *testing.T) {
	pd := &mockPD{emails: map[string]string{
		"PSCHED1": "jane@example.com",
		"PSCHED2": "bob@example.com",
	}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.users["bob@example.com"] = "U456"

	s := &Syncer{
		PD:    pd,
		Slack: sl,
		Mappings: []config.Mapping{
			{PagerDutyScheduleID: "PSCHED1", SlackUserGroupID: "S111"},
			{PagerDutyScheduleID: "PSCHED2", SlackUserGroupID: "S222"},
		},
		Logger: slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if users := sl.updatedGroups["S111"]; len(users) != 1 || users[0] != "U123" {
		t.Errorf("S111 users = %v, want [U123]", users)
	}
	if users := sl.updatedGroups["S222"]; len(users) != 1 || users[0] != "U456" {
		t.Errorf("S222 users = %v, want [U456]", users)
	}
}

func TestRun_AbortsOnFirstError(t *testing.T) {
	pd := &mockPD{emails: map[string]string{
		"PSCHED1": "jane@example.com",
		// PSCHED2 missing — will error
	}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"

	s := &Syncer{
		PD:    pd,
		Slack: sl,
		Mappings: []config.Mapping{
			{PagerDutyScheduleID: "PSCHED1", SlackUserGroupID: "S111"},
			{PagerDutyScheduleID: "PSCHED2", SlackUserGroupID: "S222"},
			{PagerDutyScheduleID: "PSCHED1", SlackUserGroupID: "S333"},
		},
		Logger: slog.Default(),
	}

	err := s.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// First mapping should have succeeded.
	if users := sl.updatedGroups["S111"]; len(users) != 1 || users[0] != "U123" {
		t.Errorf("S111 should have succeeded, got %v", users)
	}
	// Third mapping should never have been attempted.
	if _, ok := sl.updatedGroups["S333"]; ok {
		t.Error("S333 should not have been attempted after S222 failed")
	}
}

func TestRun_GetMembersError_Aborts(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.membersErr = errors.New("connection reset")

	s := &Syncer{
		PD:       pd,
		Slack:    sl,
		Mappings: []config.Mapping{{PagerDutyScheduleID: "P1", SlackUserGroupID: "S1"}},
		Logger:   slog.Default(),
	}

	err := s.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when GetUserGroupMembers fails, got nil")
	}
	// Should not have updated the group.
	if _, ok := sl.updatedGroups["S1"]; ok {
		t.Error("should not update usergroup when member fetch fails")
	}
	// Should not have sent any messages.
	if len(sl.messages) != 0 {
		t.Errorf("should not send messages when member fetch fails, got %v", sl.messages)
	}
}

func TestRun_PDError(t *testing.T) {
	pd := &mockPD{err: errors.New("pd failure")}
	sl := newMockSlack()

	s := &Syncer{
		PD:       pd,
		Slack:    sl,
		Mappings: []config.Mapping{{PagerDutyScheduleID: "P1", SlackUserGroupID: "S1"}},
		Logger:   slog.Default(),
	}

	if err := s.Run(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_SlackLookupError(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.lookupErr = errors.New("not found")

	s := &Syncer{
		PD:       pd,
		Slack:    sl,
		Mappings: []config.Mapping{{PagerDutyScheduleID: "P1", SlackUserGroupID: "S1"}},
		Logger:   slog.Default(),
	}

	if err := s.Run(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_SlackUpdateError(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.updateErr = errors.New("update failed")

	s := &Syncer{
		PD:       pd,
		Slack:    sl,
		Mappings: []config.Mapping{{PagerDutyScheduleID: "P1", SlackUserGroupID: "S1"}},
		Logger:   slog.Default(),
	}

	if err := s.Run(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_DMSentOnUserChange(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.groupMembers["S1"] = []string{"U999"} // different user currently in group

	s := &Syncer{
		PD:       pd,
		Slack:    sl,
		Mappings: []config.Mapping{{PagerDutyScheduleID: "P1", SlackUserGroupID: "S1"}},
		Logger:   slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgs := sl.messages["U123"]; len(msgs) != 1 {
		t.Errorf("DMs to U123 = %v, want 1 message", msgs)
	}
}

func TestRun_NoDMWhenUserUnchanged(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.groupMembers["S1"] = []string{"U123"} // same user already in group

	s := &Syncer{
		PD:       pd,
		Slack:    sl,
		Mappings: []config.Mapping{{PagerDutyScheduleID: "P1", SlackUserGroupID: "S1"}},
		Logger:   slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sl.messages) != 0 {
		t.Errorf("messages = %v, want none", sl.messages)
	}
	// Should skip the update entirely when user is unchanged.
	if _, ok := sl.updatedGroups["S1"]; ok {
		t.Error("expected no usergroup update when on-call user is unchanged")
	}
}

func TestRun_MessageFailureDoesNotFailSync(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.groupMembers["S1"] = []string{"U999"}
	sl.msgErr = errors.New("message failed")

	s := &Syncer{
		PD:       pd,
		Slack:    sl,
		Mappings: []config.Mapping{{PagerDutyScheduleID: "P1", SlackUserGroupID: "S1"}},
		Logger:   slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("message failure should not cause sync error, got: %v", err)
	}
	if users := sl.updatedGroups["S1"]; len(users) != 1 || users[0] != "U123" {
		t.Errorf("updatedGroups[S1] = %v, want [U123]", users)
	}
}

func TestRun_ChannelNotificationOnChange(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.groupMembers["S1"] = []string{"U999"} // different user — triggers notification

	s := &Syncer{
		PD:    pd,
		Slack: sl,
		Mappings: []config.Mapping{{
			PagerDutyScheduleID: "P1",
			SlackUserGroupID:    "S1",
			SlackChannelID:      "C_TEAM",
			NotificationMessage: "{@user} is now on-call!",
		}},
		Logger: slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have DM to user and message to channel.
	if msgs := sl.messages["U123"]; len(msgs) != 1 {
		t.Errorf("DMs to U123 = %v, want 1", msgs)
	}
	channelMsgs := sl.messages["C_TEAM"]
	if len(channelMsgs) != 1 {
		t.Fatalf("channel messages = %v, want 1", channelMsgs)
	}
	want := "<@U123> is now on-call!"
	if channelMsgs[0] != want {
		t.Errorf("channel message = %q, want %q", channelMsgs[0], want)
	}
}

func TestRun_NoChannelNotificationWhenUnchanged(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.groupMembers["S1"] = []string{"U123"} // same user — no notification

	s := &Syncer{
		PD:    pd,
		Slack: sl,
		Mappings: []config.Mapping{{
			PagerDutyScheduleID: "P1",
			SlackUserGroupID:    "S1",
			SlackChannelID:      "C_TEAM",
			NotificationMessage: "{@user} is now on-call!",
		}},
		Logger: slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sl.messages) != 0 {
		t.Errorf("messages = %v, want none", sl.messages)
	}
	if _, ok := sl.updatedGroups["S1"]; ok {
		t.Error("expected no usergroup update when on-call user is unchanged")
	}
}

func TestRun_NoChannelNotificationWhenNotConfigured(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.groupMembers["S1"] = []string{"U999"} // user changed

	s := &Syncer{
		PD:    pd,
		Slack: sl,
		Mappings: []config.Mapping{{
			PagerDutyScheduleID: "P1",
			SlackUserGroupID:    "S1",
			// No SlackChannelID or NotificationMessage configured.
		}},
		Logger: slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// DM should still be sent, but no channel message.
	if msgs := sl.messages["U123"]; len(msgs) != 1 {
		t.Errorf("DMs to U123 = %v, want 1", msgs)
	}
	if _, ok := sl.messages["C_TEAM"]; ok {
		t.Error("expected no channel message when not configured")
	}
}
