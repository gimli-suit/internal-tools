# pd-rotation-slack-sync

Syncs PagerDuty on-call rotations to Slack user groups. For each configured mapping, the tool looks up who is currently on-call in PagerDuty, resolves their Slack user by email, and updates the corresponding Slack user group membership.

## How it works

1. For each mapping in `config.json`:
   - Queries the PagerDuty API for the current on-call user on the given schedule
   - Looks up the on-call user's email in Slack
   - Checks the current members of the Slack user group
   - Updates the Slack user group to contain that user
   - If the on-call user changed, sends them a DM notifying them they've been added to the group

## Setup

### 1. Create API tokens

**PagerDuty:**

- Go to PagerDuty > Integrations > API Access Keys
- Create a read-only REST API key

**Slack:**

- Create a Slack app (or use an existing one) with the following bot token scopes:
  - `users:read.email` — look up users by email
  - `users:read` — read user profiles
  - `usergroups:read` — read user group membership
  - `usergroups:write` — update user group membership
  - `chat:write` — send DM notifications to on-call users
- Install the app to your workspace and copy the bot token (`xoxb-...`)

### 2. Configure environment

Copy the example files and fill in your values:

```sh
cp .env.example .env
cp config.json.example config.json
```

**.env** — set your API tokens:

```
PAGERDUTY_API_TOKEN=your-pagerduty-api-token
SLACK_API_TOKEN=xoxb-your-slack-bot-token
```

**config.json** — define schedule-to-usergroup mappings:

```json
{
  "mappings": [
    {
      "pagerduty_schedule_id": "P2H5CBI",
      "slack_usergroup_id": "S0AV190BG8H"
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `pagerduty_schedule_id` | PagerDuty schedule ID (found in the schedule URL) |
| `slack_usergroup_id` | Slack user group ID to update with the on-call user |

You can add multiple mappings to sync several rotations at once.

### 3. Build and run

```sh
go build -o pd-slack-sync .
./pd-slack-sync
```

## Development

Run tests:

```sh
go test ./...
```
