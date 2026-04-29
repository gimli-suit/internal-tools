package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/github-deployed-issue-sync/internal/config"
	"github.com/github-deployed-issue-sync/internal/ghauth"
	"github.com/github-deployed-issue-sync/internal/github"
	"github.com/github-deployed-issue-sync/internal/prodver"
	"github.com/github-deployed-issue-sync/internal/sync"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	httpClient := &http.Client{Timeout: 15 * time.Second}

	ghToken, err := ghauth.GetInstallationToken(ctx, httpClient, "https://api.github.com", cfg.GitHubAppID, cfg.GitHubAppInstallationID, cfg.GitHubAppPrivateKey)
	if err != nil {
		slog.Error("github app authentication failed", "error", err)
		os.Exit(1)
	}

	prodverClient := &prodver.Client{
		HTTPClient: httpClient,
		URL:        cfg.ProdverURL,
		ShardName:  cfg.ShardName,
	}

	ghClient := &github.Client{
		HTTPClient:    httpClient,
		Token:         ghToken,
		GraphQLURL:    "https://api.github.com/graphql",
		RestBaseURL:   "https://api.github.com",
		Org:           cfg.GitHubOrg,
		ProjectNumber: cfg.ProjectNumber,
	}

	syncer := &sync.Syncer{
		Prodver:         prodverClient,
		ProjectQuerier:  ghClient,
		AncestorChecker: ghClient,
		StatusUpdater:   ghClient,
		Org:             cfg.GitHubOrg,
		Repo:            cfg.GitHubRepo,
		Logger:          slog.Default(),
	}

	if err := syncer.Run(ctx); err != nil {
		slog.Error("sync failed", "error", err)
		os.Exit(1)
	}
}
