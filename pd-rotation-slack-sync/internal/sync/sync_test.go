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
	dmErr     error

	groupMembers  map[string][]string // group ID -> current member IDs
	membersErr    error
	updatedGroups map[string][]string // group ID -> user IDs
	dmsSent       []string            // user IDs that received DMs
}

func newMockSlack() *mockSlack {
	return &mockSlack{
		users:         make(map[string]string),
		groupMembers:  make(map[string][]string),
		updatedGroups: make(map[string][]string),
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

func (m *mockSlack) SendDM(_ context.Context, userID, text string) error {
	if m.dmErr != nil {
		return m.dmErr
	}
	m.dmsSent = append(m.dmsSent, userID)
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

func TestRun_PartialFailure(t *testing.T) {
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
		},
		Logger: slog.Default(),
	}

	err := s.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for partial failure, got nil")
	}

	// First mapping should still have succeeded
	if users := sl.updatedGroups["S111"]; len(users) != 1 || users[0] != "U123" {
		t.Errorf("S111 should have succeeded, got %v", users)
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
	if len(sl.dmsSent) != 1 || sl.dmsSent[0] != "U123" {
		t.Errorf("dmsSent = %v, want [U123]", sl.dmsSent)
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
	if len(sl.dmsSent) != 0 {
		t.Errorf("dmsSent = %v, want none", sl.dmsSent)
	}
}

func TestRun_DMFailureDoesNotFailSync(t *testing.T) {
	pd := &mockPD{emails: map[string]string{"P1": "jane@example.com"}}
	sl := newMockSlack()
	sl.users["jane@example.com"] = "U123"
	sl.groupMembers["S1"] = []string{"U999"}
	sl.dmErr = errors.New("DM failed")

	s := &Syncer{
		PD:       pd,
		Slack:    sl,
		Mappings: []config.Mapping{{PagerDutyScheduleID: "P1", SlackUserGroupID: "S1"}},
		Logger:   slog.Default(),
	}

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("DM failure should not cause sync error, got: %v", err)
	}
	// Group should still have been updated.
	if users := sl.updatedGroups["S1"]; len(users) != 1 || users[0] != "U123" {
		t.Errorf("updatedGroups[S1] = %v, want [U123]", users)
	}
}
