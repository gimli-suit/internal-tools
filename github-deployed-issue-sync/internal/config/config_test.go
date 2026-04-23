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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GITHUB_TOKEN"} {
		orig := os.Getenv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if orig != "" {
				os.Setenv(key, orig)
			}
		})
	}
}

const validConfig = `{
  "prodver_url": "http://prodver/control",
  "shard_name": "shard1",
  "github_org": "tailscale",
  "github_repo": "corp",
  "project_number": 42
}`

func TestLoadSuccess(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	os.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Cleanup(func() { os.Unsetenv("GITHUB_TOKEN") })

	writeFile(t, dir, "config.json", validConfig)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHubToken != "ghp_test123" {
		t.Errorf("got token %q, want %q", cfg.GitHubToken, "ghp_test123")
	}
	if cfg.ProdverURL != "http://prodver/control" {
		t.Errorf("got prodver_url %q, want %q", cfg.ProdverURL, "http://prodver/control")
	}
	if cfg.ShardName != "shard1" {
		t.Errorf("got shard_name %q, want %q", cfg.ShardName, "shard1")
	}
	if cfg.GitHubOrg != "tailscale" {
		t.Errorf("got github_org %q, want %q", cfg.GitHubOrg, "tailscale")
	}
	if cfg.GitHubRepo != "corp" {
		t.Errorf("got github_repo %q, want %q", cfg.GitHubRepo, "corp")
	}
	if cfg.ProjectNumber != 42 {
		t.Errorf("got project_number %d, want %d", cfg.ProjectNumber, 42)
	}
}

func TestLoadMissingToken(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	writeFile(t, dir, "config.json", validConfig)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing GITHUB_TOKEN")
	}
}

func TestLoadMissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	os.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Cleanup(func() { os.Unsetenv("GITHUB_TOKEN") })

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing config.json")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	os.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Cleanup(func() { os.Unsetenv("GITHUB_TOKEN") })

	writeFile(t, dir, "config.json", "not json")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadMissingFields(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	os.Setenv("GITHUB_TOKEN", "ghp_test123")
	t.Cleanup(func() { os.Unsetenv("GITHUB_TOKEN") })

	writeFile(t, dir, "config.json", `{"prodver_url": "http://prodver/control"}`)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestLoadEnvFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	writeFile(t, dir, ".env", "GITHUB_TOKEN=from_file")
	writeFile(t, dir, "config.json", validConfig)

	// Set env var — should take precedence over .env file
	os.Setenv("GITHUB_TOKEN", "from_env")
	t.Cleanup(func() { os.Unsetenv("GITHUB_TOKEN") })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHubToken != "from_env" {
		t.Errorf("got token %q, want %q (env should take precedence)", cfg.GitHubToken, "from_env")
	}
}

func TestLoadFromEnvFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	writeFile(t, dir, ".env", "GITHUB_TOKEN=from_dotenv")
	writeFile(t, dir, "config.json", validConfig)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHubToken != "from_dotenv" {
		t.Errorf("got token %q, want %q", cfg.GitHubToken, "from_dotenv")
	}
}
