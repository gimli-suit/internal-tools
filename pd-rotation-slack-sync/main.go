package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/pd-rotation-slack-sync/internal/config"
	"github.com/pd-rotation-slack-sync/internal/pagerduty"
	"github.com/pd-rotation-slack-sync/internal/slack"
	"github.com/pd-rotation-slack-sync/internal/sync"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	httpClient := &http.Client{Timeout: 10 * time.Second}

	pdClient := &pagerduty.Client{
		HTTPClient: httpClient,
		APIToken:   cfg.PagerDutyToken,
		BaseURL:    "https://api.pagerduty.com",
	}

	slackClient := &slack.Client{
		HTTPClient: httpClient,
		APIToken:   cfg.SlackToken,
		BaseURL:    "https://slack.com/api",
	}

	syncer := &sync.Syncer{
		PD:       pdClient,
		Slack:    slackClient,
		Mappings: cfg.Mappings,
		Logger:   slog.Default(),
	}

	if err := syncer.Run(ctx); err != nil {
		slog.Error("sync failed", "error", err)
		os.Exit(1)
	}
}
