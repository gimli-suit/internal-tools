package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"net/http"
	"os"
	"time"

	"github.com/github-deployed-issue-sync/internal/config"
	"github.com/github-deployed-issue-sync/internal/ghauth"
	"github.com/github-deployed-issue-sync/internal/github"
	"github.com/github-deployed-issue-sync/internal/prodver"
	"github.com/github-deployed-issue-sync/internal/sync"
)

func checkInstallationAuth(ctx context.Context, httpClient *http.Client, token string, cfg *config.Config) {
	fmt.Println("=== GitHub App Authentication Check ===")
	fmt.Printf("App ID: %d\n", cfg.GitHubAppID)
	fmt.Printf("Installation ID: %d\n", cfg.GitHubAppInstallationID)
	fmt.Println("Token: OK (obtained successfully)")

	// Check installation details.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/installation/repositories?per_page=5", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("Repos check failed: %v\n", err)
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("\n=== Accessible Repositories (first 5) ===\n")
		fmt.Printf("Status: %d\n", resp.StatusCode)
		var repos struct {
			TotalCount int `json:"total_count"`
			Repositories []struct {
				FullName string `json:"full_name"`
			} `json:"repositories"`
		}
		if json.Unmarshal(body, &repos) == nil {
			fmt.Printf("Total: %d\n", repos.TotalCount)
			for _, r := range repos.Repositories {
				fmt.Printf("  - %s\n", r.FullName)
			}
		} else {
			fmt.Printf("Response: %s\n", body)
		}
	}

	// Check org project access via GraphQL.
	queries := []struct {
		name  string
		query string
	}{
		{
			"Project Access",
			fmt.Sprintf(`{"query":"query { organization(login: \"%s\") { projectV2(number: %d) { id title } } }"}`, cfg.GitHubOrg, cfg.ProjectNumber),
		},
		{
			"Project Items (first 20, no timeline)",
			fmt.Sprintf(`{"query":"query { organization(login: \"%s\") { projectV2(number: %d) { items(first: 20) { nodes { id content { ... on Issue { number title repository { nameWithOwner } } } } } } } }"}`, cfg.GitHubOrg, cfg.ProjectNumber),
		},
		{
			"Direct repo issues (tailscale/corp)",
			fmt.Sprintf(`{"query":"query { repository(owner: \"%s\", name: \"%s\") { issues(first: 3, orderBy: {field: CREATED_AT, direction: DESC}) { nodes { number title } } } }"}`, cfg.GitHubOrg, cfg.GitHubRepo),
		},
		{
			"Iteration field configuration",
			fmt.Sprintf(`{"query":"query { organization(login: \"%s\") { projectV2(number: %d) { field(name: \"Iteration\") { ... on ProjectV2IterationField { id configuration { iterations { title startDate duration } completedIterations { title startDate duration } } } } } } }"}`, cfg.GitHubOrg, cfg.ProjectNumber),
		},
	}

	for _, q := range queries {
		fmt.Printf("\n=== %s ===\n", q.name)
		req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", io.NopCloser(strings.NewReader(q.query)))
		req2.Header.Set("Authorization", "Bearer "+token)
		req2.Header.Set("Content-Type", "application/json")
		resp2, err := httpClient.Do(req2)
		if err != nil {
			fmt.Printf("GraphQL check failed: %v\n", err)
			continue
		}
		defer resp2.Body.Close()
		body2, _ := io.ReadAll(resp2.Body)
		var pretty json.RawMessage
		if json.Unmarshal(body2, &pretty) == nil {
			formatted, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Println(string(formatted))
		} else {
			fmt.Println(string(body2))
		}
	}
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print issues that would be updated without making changes")
	checkAuth := flag.Bool("check-auth", false, "authenticate and print installation token scopes, then exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	httpClient := &http.Client{Timeout: 15 * time.Second}

	ghToken, permissions, err := ghauth.GetInstallationToken(ctx, httpClient, "https://api.github.com", cfg.GitHubAppID, cfg.GitHubAppInstallationID, cfg.GitHubAppPrivateKey)
	if err != nil {
		slog.Error("github app authentication failed", "error", err)
		os.Exit(1)
	}

	if *checkAuth {
		fmt.Println("\n=== Token Permissions ===")
		for k, v := range permissions {
			fmt.Printf("  %s: %s\n", k, v)
		}
		checkInstallationAuth(ctx, httpClient, ghToken, cfg)
		return
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
		Prodver:          prodverClient,
		ProjectQuerier:   ghClient,
		AncestorChecker:  ghClient,
		StatusUpdater:    ghClient,
		IterationUpdater: ghClient,
		Org:              cfg.GitHubOrg,
		Repo:             cfg.GitHubRepo,
		DryRun:           *dryRun,
		Logger:           slog.Default(),
	}

	if err := syncer.Run(ctx); err != nil {
		slog.Error("sync failed", "error", err)
		os.Exit(1)
	}
}
