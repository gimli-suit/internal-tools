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

### 1. Create a GitHub App

1. Go to your organization's Settings > Developer settings > GitHub Apps > New GitHub App
2. Fill in a name and homepage URL (can be any valid URL)
3. Uncheck **Active** under Webhook (this tool doesn't use webhooks)
4. Set the following permissions:
   - **Repository permissions:** Contents → Read-only
   - **Organization permissions:** Projects → Read and write
5. Under "Where can this GitHub App be installed?", select **Only on this account**
6. Click **Create GitHub App**
7. On the app's General page, note the **App ID** (displayed near the top)
8. Scroll to **Private keys** and click **Generate a private key** — this downloads a `.pem` file

### 2. Install the GitHub App on your organization

1. From your app's settings page, click **Install App** in the left sidebar
2. Click **Install** next to your organization
3. Choose **Only select repositories** and pick the target repository (e.g., `corp`)
4. Click **Install**
5. After installation, you'll be redirected to a URL like:
   ```
   https://github.com/organizations/tailscale/settings/installations/78901234
   ```
   The number at the end (`78901234`) is your **Installation ID** — save this for configuration

### 3. Configure environment

Copy the example files and fill in your values:

```sh
cp .env.example .env
cp config.json.example config.json
```

**.env** — set your GitHub App credentials:

```
GITHUB_APP_ID=123456
GITHUB_APP_INSTALLATION_ID=78901234
GITHUB_APP_PRIVATE_KEY_PATH=/path/to/your-app.private-key.pem
```

| Variable | Description |
|----------|-------------|
| `GITHUB_APP_ID` | The App ID from your GitHub App's settings page |
| `GITHUB_APP_INSTALLATION_ID` | The Installation ID (from the URL after installing the app on your org) |
| `GITHUB_APP_PRIVATE_KEY_PATH` | Path to the `.pem` private key file downloaded from the app settings |
| `GITHUB_APP_PRIVATE_KEY` | Alternative: raw PEM content (useful in CI/containers). Takes precedence over `_PATH` if both are set. |

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

### 4. Build and run

```sh
go build -o github-deployed-issue-sync .
./github-deployed-issue-sync
```

## Development

Run tests:

```sh
go test ./...
```
