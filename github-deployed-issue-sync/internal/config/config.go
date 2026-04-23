package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	GitHubToken   string
	ProdverURL    string
	ShardName     string
	GitHubOrg     string
	GitHubRepo    string
	ProjectNumber int
}

type configFile struct {
	ProdverURL    string `json:"prodver_url"`
	ShardName     string `json:"shard_name"`
	GitHubOrg     string `json:"github_org"`
	GitHubRepo    string `json:"github_repo"`
	ProjectNumber int    `json:"project_number"`
}

// Load reads the GitHub token from .env/environment and settings from config.json.
func Load() (*Config, error) {
	loadEnvFile(".env")

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("missing required environment variable: GITHUB_TOKEN")
	}

	cf, err := loadConfigFile("config.json")
	if err != nil {
		return nil, err
	}

	return &Config{
		GitHubToken:   token,
		ProdverURL:    cf.ProdverURL,
		ShardName:     cf.ShardName,
		GitHubOrg:     cf.GitHubOrg,
		GitHubRepo:    cf.GitHubRepo,
		ProjectNumber: cf.ProjectNumber,
	}, nil
}

func loadConfigFile(path string) (*configFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cf configFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	var missing []string
	if cf.ProdverURL == "" {
		missing = append(missing, "prodver_url")
	}
	if cf.ShardName == "" {
		missing = append(missing, "shard_name")
	}
	if cf.GitHubOrg == "" {
		missing = append(missing, "github_org")
	}
	if cf.GitHubRepo == "" {
		missing = append(missing, "github_repo")
	}
	if cf.ProjectNumber == 0 {
		missing = append(missing, "project_number")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("config file %s missing required fields: %s", path, strings.Join(missing, ", "))
	}

	return &cf, nil
}

// loadEnvFile reads a .env file and sets any variables not already present
// in the environment. Real env vars always take precedence.
// If the file doesn't exist, this is a no-op.
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}
