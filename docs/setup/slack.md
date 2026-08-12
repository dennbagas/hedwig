# Slack Setup

This is a one-time, operational (not code) setup to get the values `slack.*` config needs. Skip this entirely if you're only using Telegram (`slack.enabled: false`).

Slack notifications work on their own. If you also want the interactive CI/CD retry button (`retry.enabled: true`), the app additionally needs Interactivity turned on (step 4) — leave `retry.enabled: false` and you can skip that step.

## 1. Create the app

Create a Slack app at [api.slack.com/apps](https://api.slack.com/apps) (from scratch or a manifest) in the target workspace.

## 2. Bot token

Under **OAuth & Permissions**:

1. Add the `chat:write` Bot Token Scope (add `chat:write.public` too if you don't want to invite the bot to the channel first).
2. Install the app to the workspace.
3. Copy the **Bot User OAuth Token** (starts with `xoxb-`) — this is `slack.bot_token` / `APP_SLACK_BOT_TOKEN`.

## 3. Signing secret

Under **Basic Information**, copy the **Signing Secret** — this is `slack.signing_secret` / `APP_SLACK_SIGNING_SECRET`. Hedwig uses it to verify that interaction requests actually came from Slack (HMAC-SHA256 signature check on `slack.webhook_path`).

## 4. Interactivity (only needed for the retry button)

Under **Interactivity & Shortcuts**:

1. Turn Interactivity on.
2. Set the **Request URL** to `https://<your-domain><slack.webhook_path>` — your public HTTPS domain plus the `slack.webhook_path` from config (default `/webhooks/slack/interactions`). Slack sends the CI/CD retry button's tap events here.

If you leave `retry.enabled: false`, you can skip this step — Slack notifications are sent the same way either way, this only matters for the button's tap events.

## 5. Channel ID

1. Invite the bot to the channel you want notifications in: `/invite @your-bot-name` in that channel.
2. Get the channel's ID: right-click the channel → **View channel details** → the ID is shown at the bottom of that panel (starts with `C`, e.g. `C0123456789`). This is `slack.channel_id`.

## Result

You should now have four values for the `slack:` section of `config.yaml`:

```yaml
slack:
  enabled: true
  bot_token: ""          # set via APP_SLACK_BOT_TOKEN, step 2
  signing_secret: ""     # set via APP_SLACK_SIGNING_SECRET, step 3
  channel_id: "C0123456789"   # step 5
  webhook_path: /webhooks/slack/interactions
```

After changing `slack.enabled` or any of the above, restart/redeploy Hedwig to pick up the change (in Kubernetes: `kubectl rollout restart deployment/hedwig -n <namespace>`).
