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

	updatedGroups map[string][]string // group ID -> user IDs
}

func newMockSlack() *mockSlack {
	return &mockSlack{
		users:         make(map[string]string),
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

func (m *mockSlack) UpdateUserGroupMembers(_ context.Context, groupID string, userIDs []string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updatedGroups[groupID] = userIDs
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
