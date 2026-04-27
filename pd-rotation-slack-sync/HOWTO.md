# How To Use PD Rotation Slack Sync

## Problem To Be Solved

As our teams grow, it can become increasingly difficult to identify who on a team is the person of contact. Especially during an incident, jumping through tools to search for teams and find who is on call is precious time wasted. Wouldn't it be great if we always had a single resource that we could reach out to and it always linked to the right person?

Enter Pagerduty Rotation Slack Sync! This Slack app will regularly sync people who are on call with their team's contact slack handle. This way, folks simply have to remember to ping one group for each team and it will always ping the person on call.

## How should we use this?

Ultimately, this will give us a single user group per team that can be used by anyone when we have an escalation or incident that requires pinging a given team. This role will be called the team's: **Best Go-To Person (BGP)**. In order to participate, here are the steps to follow:

1. Create a Pagerduty schedule for your team and assign it to an escalation policy
2. Create a Slack user group for your team with template: `TEAM-NAME-BGP` (BGP stands for Best Go-To Person)
3. Identify which of your slack channels you'd like to have the app post to when changes happen
4. Add a config block to the Slack app for their team with the following:

```json
    {
      "team": "identity",
      "pagerduty_schedule_id": "P2XXXXX",
      "slack_usergroup_id": "SXXXXXXXXXX",
      "slack_channel_id": "CXXXXXXXXXX",
      "notification_message": "{@user} is now on-call for Team 1!" 
    },
```

- **team**: The name of the team (using Identity as example)
- **pagerduty_schedule_id**: ID of the team's schedule
- **slack_usergroup_id**: ID of the Slack group with name: @TEAM-bgp
- **slack_channel_id**: ID of the Slack channel for the App to post updates to on rotation changes
- **notification_message**: The custom message to send to the channel listed above

Then, periodically the app will run and update the user group with the current on-call engineer for the team (`@TEAM-bgp`). When the app detects a change in on-call rotation, it will:

- Post an update to the configured slack channel with who is now on call
- Send a DM to the new person that they are now the go-to person for the team

Finally, anytime someone wants to escalate to a team, they simply need to ping: `@TEAM-BGP` and that will ping the person who is on-call for the team
