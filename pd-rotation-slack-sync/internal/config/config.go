package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Mapping struct {
	PagerDutyScheduleID string `json:"pagerduty_schedule_id"`
	SlackUserGroupID    string `json:"slack_usergroup_id"`
}

type Config struct {
	PagerDutyToken string
	SlackToken     string
	Mappings       []Mapping
}

type configFile struct {
	Mappings []Mapping `json:"mappings"`
}

// Load reads tokens from .env/environment and mappings from config.json.
func Load() (*Config, error) {
	loadEnvFile(".env")

	cfg := &Config{
		PagerDutyToken: os.Getenv("PAGERDUTY_API_TOKEN"),
		SlackToken:     os.Getenv("SLACK_API_TOKEN"),
	}

	var missing []string
	if cfg.PagerDutyToken == "" {
		missing = append(missing, "PAGERDUTY_API_TOKEN")
	}
	if cfg.SlackToken == "" {
		missing = append(missing, "SLACK_API_TOKEN")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	mappings, err := loadMappings("config.json")
	if err != nil {
		return nil, err
	}
	cfg.Mappings = mappings

	return cfg, nil
}

func loadMappings(path string) ([]Mapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cf configFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	if len(cf.Mappings) == 0 {
		return nil, fmt.Errorf("config file %s has no mappings", path)
	}

	for i, m := range cf.Mappings {
		if m.PagerDutyScheduleID == "" {
			return nil, fmt.Errorf("mapping %d: missing pagerduty_schedule_id", i)
		}
		if m.SlackUserGroupID == "" {
			return nil, fmt.Errorf("mapping %d: missing slack_usergroup_id", i)
		}
	}

	return cf.Mappings, nil
}

// loadEnvFile reads a .env file and sets any variables not already present
// in the environment. This means real env vars always take precedence.
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
		// Strip surrounding quotes
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		// Only set if not already in env (env vars take precedence)
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}
