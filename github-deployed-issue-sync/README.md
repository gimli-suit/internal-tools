# github-deployed-issue-sync

Automatically marks GitHub Project V2 issues as "🚢 Shipped" when their linked PRs have been deployed to production.

The tool checks the deployed commit SHA from [prodver](http://prodver), then cross-references it against merged PRs linked to issues in a GitHub Project. If all of an issue's closing PRs (in the target repo) are ancestors of the deployed SHA, the issue's status is updated to "🚢 Shipped".

## How it works

1. Fetches the prodver page and parses the deployed `tailscale/corp` commit SHA for the configured shard
2. Queries all items in the configured GitHub Project V2 via GraphQL
3. For each issue that isn't already shipped and has all linked PRs merged:
   - Uses the GitHub compare API to check if each PR's merge commit is an ancestor of the deployed SHA
   - If all corp PRs are deployed, updates the project status to "🚢 Shipped"

## Setup

### 1. Create a GitHub Personal Access Token

**Fine-grained token** (preferred):

- Go to Settings > Developer settings > Personal access tokens > Fine-grained tokens
- Set resource owner to `tailscale`
- Repository access: select `tailscale/corp` (or the target repo)
- Repository permissions: **Contents** → Read-only
- Organization permissions: **Projects** → Read and write

**Classic token** (if fine-grained isn't available):

- Scopes: `repo`, `project`

### 2. Configure environment

Copy the example files and fill in your values:

```sh
cp .env.example .env
cp config.json.example config.json
```

**.env** — set your GitHub token:

```
GITHUB_TOKEN=ghp_your_token_here
```

**config.json** — configure the sync target:

```json
{
  "prodver_url": "http://prodver/control",
  "shard_name": "shard1",
  "github_org": "tailscale",
  "github_repo": "corp",
  "project_number": 42
}
```

| Field | Description |
|-------|-------------|
| `prodver_url` | URL of the prodver page to scrape (e.g., `http://prodver/control`) |
| `shard_name` | Which shard row to read the deployed SHA from (e.g., `shard1`) |
| `github_org` | GitHub organization that owns the project |
| `github_repo` | Repository to check merge commits against |
| `project_number` | Project V2 number (from the project URL) |

### 3. Build and run

```sh
go build -o github-deployed-issue-sync .
./github-deployed-issue-sync
```

## Development

Run tests:

```sh
go test ./...
```
