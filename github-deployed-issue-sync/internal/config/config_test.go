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

var appEnvVars = []string{
	"GITHUB_APP_ID",
	"GITHUB_APP_INSTALLATION_ID",
	"GITHUB_APP_PRIVATE_KEY",
	"GITHUB_APP_PRIVATE_KEY_PATH",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range appEnvVars {
		orig := os.Getenv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if orig != "" {
				os.Setenv(key, orig)
			}
		})
	}
}

func setAppEnv(t *testing.T) {
	t.Helper()
	os.Setenv("GITHUB_APP_ID", "12345")
	os.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
	os.Setenv("GITHUB_APP_PRIVATE_KEY", "fake-pem-content")
	t.Cleanup(func() {
		for _, key := range appEnvVars {
			os.Unsetenv(key)
		}
	})
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
	setAppEnv(t)

	writeFile(t, dir, "config.json", validConfig)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHubAppID != 12345 {
		t.Errorf("got app ID %d, want %d", cfg.GitHubAppID, 12345)
	}
	if cfg.GitHubAppInstallationID != 67890 {
		t.Errorf("got installation ID %d, want %d", cfg.GitHubAppInstallationID, 67890)
	}
	if string(cfg.GitHubAppPrivateKey) != "fake-pem-content" {
		t.Errorf("got private key %q, want %q", cfg.GitHubAppPrivateKey, "fake-pem-content")
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

func TestLoadMissingAppID(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	os.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
	os.Setenv("GITHUB_APP_PRIVATE_KEY", "fake-pem")
	t.Cleanup(func() {
		os.Unsetenv("GITHUB_APP_INSTALLATION_ID")
		os.Unsetenv("GITHUB_APP_PRIVATE_KEY")
	})

	writeFile(t, dir, "config.json", validConfig)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing GITHUB_APP_ID")
	}
}

func TestLoadMissingInstallationID(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	os.Setenv("GITHUB_APP_ID", "12345")
	os.Setenv("GITHUB_APP_PRIVATE_KEY", "fake-pem")
	t.Cleanup(func() {
		os.Unsetenv("GITHUB_APP_ID")
		os.Unsetenv("GITHUB_APP_PRIVATE_KEY")
	})

	writeFile(t, dir, "config.json", validConfig)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing GITHUB_APP_INSTALLATION_ID")
	}
}

func TestLoadMissingPrivateKey(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	os.Setenv("GITHUB_APP_ID", "12345")
	os.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
	t.Cleanup(func() {
		os.Unsetenv("GITHUB_APP_ID")
		os.Unsetenv("GITHUB_APP_INSTALLATION_ID")
	})

	writeFile(t, dir, "config.json", validConfig)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
}

func TestLoadInvalidAppID(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	os.Setenv("GITHUB_APP_ID", "not-a-number")
	os.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
	os.Setenv("GITHUB_APP_PRIVATE_KEY", "fake-pem")
	t.Cleanup(func() {
		for _, key := range appEnvVars {
			os.Unsetenv(key)
		}
	})

	writeFile(t, dir, "config.json", validConfig)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid GITHUB_APP_ID")
	}
}

func TestLoadPrivateKeyFromFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	keyPath := filepath.Join(dir, "app.pem")
	if err := os.WriteFile(keyPath, []byte("pem-from-file"), 0600); err != nil {
		t.Fatal(err)
	}

	os.Setenv("GITHUB_APP_ID", "12345")
	os.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
	os.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", keyPath)
	t.Cleanup(func() {
		for _, key := range appEnvVars {
			os.Unsetenv(key)
		}
	})

	writeFile(t, dir, "config.json", validConfig)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(cfg.GitHubAppPrivateKey) != "pem-from-file" {
		t.Errorf("got private key %q, want %q", cfg.GitHubAppPrivateKey, "pem-from-file")
	}
}

func TestLoadDirectKeyTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	keyPath := filepath.Join(dir, "app.pem")
	if err := os.WriteFile(keyPath, []byte("pem-from-file"), 0600); err != nil {
		t.Fatal(err)
	}

	os.Setenv("GITHUB_APP_ID", "12345")
	os.Setenv("GITHUB_APP_INSTALLATION_ID", "67890")
	os.Setenv("GITHUB_APP_PRIVATE_KEY", "pem-direct")
	os.Setenv("GITHUB_APP_PRIVATE_KEY_PATH", keyPath)
	t.Cleanup(func() {
		for _, key := range appEnvVars {
			os.Unsetenv(key)
		}
	})

	writeFile(t, dir, "config.json", validConfig)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(cfg.GitHubAppPrivateKey) != "pem-direct" {
		t.Errorf("got private key %q, want %q (direct should take precedence)", cfg.GitHubAppPrivateKey, "pem-direct")
	}
}

func TestLoadMissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)
	setAppEnv(t)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing config.json")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)
	setAppEnv(t)

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
	setAppEnv(t)

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

	writeFile(t, dir, ".env", "GITHUB_APP_ID=111\nGITHUB_APP_INSTALLATION_ID=222\nGITHUB_APP_PRIVATE_KEY=from_file")
	writeFile(t, dir, "config.json", validConfig)

	// Set env vars — should take precedence over .env file.
	os.Setenv("GITHUB_APP_ID", "999")
	os.Setenv("GITHUB_APP_INSTALLATION_ID", "888")
	os.Setenv("GITHUB_APP_PRIVATE_KEY", "from_env")
	t.Cleanup(func() {
		for _, key := range appEnvVars {
			os.Unsetenv(key)
		}
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHubAppID != 999 {
		t.Errorf("got app ID %d, want %d (env should take precedence)", cfg.GitHubAppID, 999)
	}
}

func TestLoadFromEnvFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	clearEnv(t)

	writeFile(t, dir, ".env", "GITHUB_APP_ID=111\nGITHUB_APP_INSTALLATION_ID=222\nGITHUB_APP_PRIVATE_KEY=from_dotenv")
	writeFile(t, dir, "config.json", validConfig)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHubAppID != 111 {
		t.Errorf("got app ID %d, want %d", cfg.GitHubAppID, 111)
	}
	if string(cfg.GitHubAppPrivateKey) != "from_dotenv" {
		t.Errorf("got private key %q, want %q", cfg.GitHubAppPrivateKey, "from_dotenv")
	}
}
