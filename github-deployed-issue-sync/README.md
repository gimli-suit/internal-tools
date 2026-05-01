# github-deployed-issue-sync

Automatically updates GitHub Project V2 issues when their linked PRs have been deployed to production. The tool does two things:

1. **Marks issues as shipped** — Sets the status to "Shipped" when all linked PRs are deployed
2. **Assigns iterations** — Sets the iteration field based on when the issue was closed

## Project board requirements

The GitHub Project V2 board must be set up with the following fields and values for the app to work correctly.

### Status field

The project must have a **Status** field (single select) with at least the following options:

| Option | Purpose |
|--------|---------|
| **Done** | Issues considered complete. The app checks these for iteration assignment. |
| **Shipped** | Issues whose linked PRs have been deployed. The app sets this status automatically. |

The "Shipped" option name is matched with normalized whitespace, so "🚢 Shipped" or "🚢  Shipped" both work. Other status values (Triage, Backlog, In Progress, etc.) are ignored by the app — you can name them however you like.

### Iteration field

The project must have an **Iteration** field named exactly **"Iteration"**. The app reads the configured iterations (both active and completed) and matches issues to them by close date.

- Iterations should ideally cover contiguous date ranges with no gaps
- If an issue is closed during a gap between iterations, the app assigns it to the most recently ended iteration
- Issues closed before the earliest iteration will not be assigned an iteration

### Issues

For the app to process an issue, it must meet these criteria:

- **Added to the project board** — only issues on the board are visible to the app
- **Closed** — open issues are skipped entirely
- **Has linked PRs** — PRs must be linked via the Development sidebar (ConnectedEvent) or by referencing the issue in a PR description/comment (CrossReferencedEvent). At least one linked PR must be merged and in the target repo.
- **In the target repo or a repo the app is installed on** — issues from repos the app cannot access will appear as `null` and be skipped

### What the app will NOT do

- Overwrite an existing iteration value — if an issue already has an iteration set, the app leaves it alone
- Mark open issues as shipped — issues must be closed first
- Process issues from repos the app isn't installed on — the GitHub App must have access to the repo where the issue lives
- Close issues — the app only updates the Status and Iteration project fields

## How it works

### Shipping status

1. Fetches the deployed `tailscale/corp` commit SHA from prodver (supports both HTML and JSON endpoints)
2. Queries all items in the configured GitHub Project V2 via GraphQL
3. For each closed issue that isn't already shipped:
   - Finds linked PRs via timeline events (both `ConnectedEvent` and `CrossReferencedEvent`)
   - For merged PRs in the target repo, uses the GitHub compare API to check if the merge commit is included in the deployed SHA (works with squash merges)
   - If all corp PRs are deployed, updates the project status to "Shipped"

### Iteration assignment

After the shipping pass, the tool runs a second pass over all issues with status "Done" or "Shipped" that don't have an iteration set:

1. Reads the issue's `closedAt` date
2. Finds the iteration whose date range contains that date
3. If the close date falls in a gap between iterations, assigns the most recently ended iteration
4. Sets the iteration field on the project item

### PR detection

The tool detects linked PRs through two GitHub timeline event types:
- **ConnectedEvent** — PRs manually linked via the "Development" sidebar
- **CrossReferencedEvent** — PRs that reference the issue (merged PRs only, regardless of `willCloseTarget`)

### Caching and performance

- Ancestor check results are cached by commit SHA, so multiple issues linking the same PR only make one API call
- The tool paginates through all project items (100 per page)
- Default timeout is 15 minutes to handle large projects

## Setup

### 1. Create a GitHub App

1. Go to your organization's Settings > Developer settings > GitHub Apps > New GitHub App
2. Fill in a name and homepage URL (can be any valid URL)
3. Uncheck **Active** under Webhook (this tool doesn't use webhooks)
4. Set the following permissions:
   - **Repository permissions:**
     - Contents: Read-only
     - Issues: Read-only
     - Pull requests: Read-only
   - **Organization permissions:** Projects: Read and write
5. Under "Where can this GitHub App be installed?", select **Only on this account**
6. Click **Create GitHub App**
7. Note the **App ID** from the app's General page
8. Under **Private keys**, click **Generate a private key** — this downloads a `.pem` file

**Important:** When you update the app's permissions after initial creation, an org admin must approve the new permissions at Settings > GitHub Apps > your app before they take effect.

### 2. Install the GitHub App

1. From your app's settings page, click **Install App** in the left sidebar
2. Click **Install** next to your organization
3. Choose **Only select repositories** and pick the target repository (e.g., `corp`)
4. Click **Install**
5. Note the **Installation ID** from the URL after installation:
   ```
   https://github.com/organizations/tailscale/settings/installations/78901234
   ```

### 3. Configure

Copy the example files and fill in your values:

```sh
cp .env.example .env
cp config.json.example config.json
```

**.env** — GitHub App credentials:

```
GITHUB_APP_ID=123456
GITHUB_APP_INSTALLATION_ID=78901234
GITHUB_APP_PRIVATE_KEY_PATH=/path/to/your-app.private-key.pem
```

| Variable | Description |
|----------|-------------|
| `GITHUB_APP_ID` | App ID from the GitHub App settings page |
| `GITHUB_APP_INSTALLATION_ID` | Installation ID from the URL after installing on your org |
| `GITHUB_APP_PRIVATE_KEY_PATH` | Path to the `.pem` private key file |
| `GITHUB_APP_PRIVATE_KEY` | Alternative: raw PEM content with literal `\n` for newlines. Takes precedence over `_PATH`. |

**config.json** — sync target:

```json
{
  "prodver_url": "http://prodver/control",
  "shard_name": "control",
  "github_org": "tailscale",
  "github_repo": "corp",
  "project_number": 150
}
```

| Field | Description |
|-------|-------------|
| `prodver_url` | Prodver endpoint URL. Supports both HTML (multi-server table) and JSON (single-server) responses. |
| `shard_name` | Shard name to read the deployed SHA from (e.g., `control`, `shard1`) |
| `github_org` | GitHub organization that owns the project |
| `github_repo` | Repository to check merge commits against |
| `project_number` | Project V2 number (from the project board URL, e.g., `.../projects/150`) |

The project board must have a **Status** field (with a "Shipped" option) and an **Iteration** field.

## Usage

### Make targets

| Target | Description |
|--------|-------------|
| `make build` | Build the binary |
| `make run` | Build and run |
| `make dry-run` | Build and run in dry-run mode (logs changes without applying them, saves output to `dry-run.log`) |
| `make check-auth` | Verify GitHub App authentication, permissions, and project access |
| `make test` | Run all tests |
| `make clean` | Remove the built binary |
| `make install` | Install binary to `/usr/local/bin`, config to `/etc/github-deployed-issue-sync` |
| `make install-cron` | Install as a cron job running every 30 minutes |
| `make verify-cron` | Check that the cron job, binary, and log file are in place |
| `make uninstall-cron` | Remove the cron job |

### Flags

| Flag | Description |
|------|-------------|
| `-dry-run` | Log what would change without making any updates |
| `-check-auth` | Print token permissions, accessible repos, and test project/issue access |

### Running as a cron job

```sh
make install-cron
```

This installs the binary, sets up config at `/etc/github-deployed-issue-sync/` (with `0700` permissions), creates a log file at `/var/log/github-deployed-issue-sync/`, and adds a cron entry that runs every 30 minutes.

To verify the installation:

```sh
make verify-cron
```

### Troubleshooting

**"Resource not accessible by integration"** — The GitHub App is missing required permissions, or updated permissions haven't been approved by an org admin. Run `make check-auth` to see the actual token permissions.

**Issues returning `null` content** — The app can only read issues from repositories it's installed on. Issues from other repos in the project will show as `null` and be skipped.

**"shard not found in prodver output"** — The `shard_name` in config.json doesn't match any entry on the prodver page. The prodver endpoint may also return JSON (single-server) instead of HTML (multi-server table) — both formats are supported.

**Context deadline exceeded** — The 15-minute timeout was reached. This can happen with very large projects on the first run. Subsequent runs are faster due to caching.
