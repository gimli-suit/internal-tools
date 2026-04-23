package config

import (
	"os"
	"path/filepath"
	"testing"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func writeConfigJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Success(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	writeConfigJSON(t, dir, `{
		"mappings": [
			{"pagerduty_schedule_id": "PSCHED1", "slack_usergroup_id": "S111"},
			{"pagerduty_schedule_id": "PSCHED2", "slack_usergroup_id": "S222"}
		]
	}`)

	t.Setenv("PAGERDUTY_API_TOKEN", "pd-token")
	t.Setenv("SLACK_API_TOKEN", "xoxb-slack")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PagerDutyToken != "pd-token" {
		t.Errorf("PagerDutyToken = %q, want %q", cfg.PagerDutyToken, "pd-token")
	}
	if cfg.SlackToken != "xoxb-slack" {
		t.Errorf("SlackToken = %q, want %q", cfg.SlackToken, "xoxb-slack")
	}
	if len(cfg.Mappings) != 2 {
		t.Fatalf("got %d mappings, want 2", len(cfg.Mappings))
	}
	if cfg.Mappings[0].PagerDutyScheduleID != "PSCHED1" || cfg.Mappings[0].SlackUserGroupID != "S111" {
		t.Errorf("mapping 0 = %+v", cfg.Mappings[0])
	}
	if cfg.Mappings[1].PagerDutyScheduleID != "PSCHED2" || cfg.Mappings[1].SlackUserGroupID != "S222" {
		t.Errorf("mapping 1 = %+v", cfg.Mappings[1])
	}
}

func TestLoad_MissingTokens(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	writeConfigJSON(t, dir, `{"mappings": [{"pagerduty_schedule_id": "P1", "slack_usergroup_id": "S1"}]}`)

	t.Setenv("PAGERDUTY_API_TOKEN", "")
	t.Setenv("SLACK_API_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !contains(msg, "PAGERDUTY_API_TOKEN") || !contains(msg, "SLACK_API_TOKEN") {
		t.Errorf("error should mention both missing tokens, got: %s", msg)
	}
}

func TestLoad_MissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	t.Setenv("PAGERDUTY_API_TOKEN", "token")
	t.Setenv("SLACK_API_TOKEN", "token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoad_EmptyMappings(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	writeConfigJSON(t, dir, `{"mappings": []}`)

	t.Setenv("PAGERDUTY_API_TOKEN", "token")
	t.Setenv("SLACK_API_TOKEN", "token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for empty mappings, got nil")
	}
}

func TestLoad_InvalidMapping(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	writeConfigJSON(t, dir, `{"mappings": [{"pagerduty_schedule_id": "", "slack_usergroup_id": "S1"}]}`)

	t.Setenv("PAGERDUTY_API_TOKEN", "token")
	t.Setenv("SLACK_API_TOKEN", "token")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid mapping, got nil")
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	content := `# PagerDuty config
PAGERDUTY_API_TOKEN=from-file
SLACK_API_TOKEN='xoxb-from-file'
`
	if err := os.WriteFile(envFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PAGERDUTY_API_TOKEN", "")
	t.Setenv("SLACK_API_TOKEN", "")

	loadEnvFile(envFile)

	if got := os.Getenv("PAGERDUTY_API_TOKEN"); got != "from-file" {
		t.Errorf("PAGERDUTY_API_TOKEN = %q, want %q", got, "from-file")
	}
	if got := os.Getenv("SLACK_API_TOKEN"); got != "xoxb-from-file" {
		t.Errorf("SLACK_API_TOKEN = %q, want %q (quotes should be stripped)", got, "xoxb-from-file")
	}
}

func TestLoadEnvFile_EnvTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(envFile, []byte("MY_VAR=from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MY_VAR", "from-env")
	loadEnvFile(envFile)

	if got := os.Getenv("MY_VAR"); got != "from-env" {
		t.Errorf("MY_VAR = %q, want %q (env should take precedence)", got, "from-env")
	}
}

func TestLoadEnvFile_MissingFile(t *testing.T) {
	loadEnvFile("/nonexistent/.env")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
