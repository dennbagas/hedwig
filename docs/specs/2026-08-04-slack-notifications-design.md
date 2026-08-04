# Design: Slack as a Second Notification Destination

**Status:** approved, not yet implemented.

## Goal

Every GitHub event Hedwig currently notifies to Telegram should also notify
to Slack, including the CI/CD retry button — with retrying from either
platform updating both. Each channel (Telegram, Slack) must be independently
enable-able via config.

## Scope decisions

- **Fan-out:** every event goes to both platforms (no per-event routing).
- **Retry state:** shared across platforms — retrying on Slack strips the
  Telegram button too, and vice versa, since both buttons represent the same
  underlying CI/CD run.
- **Slack target:** a single fixed channel, configured once — same
  granularity as `telegram.chat_id` today. No per-repo channel routing.
- **Per-channel enable/disable:** both `telegram.enabled` and `slack.enabled`
  are configurable; at least one must be `true` at startup.

## Architecture

Two concrete clients (`telegrambot.Client`, new `slackbot.Client`), not a
unified `Notifier` interface. A generic interface across two platforms whose
addressing models differ (Telegram: numeric chat+message ID; Slack: channel
string + string timestamp) would need a lowest-common-denominator abstraction
today, for a third-platform scenario that isn't confirmed. Revisit only if a
third platform is actually requested — same reasoning already applied to
deferring a `Handler`/`Dispatcher` abstraction for Telegram callback routing
in `docs/plans/phase-2-pr-creation.md`.

### 1. Config

```yaml
telegram:
  enabled: true
  bot_token: "..."
  webhook_secret: "..."
  webhook_path: /webhooks/telegram
  webhook_url: https://hedwig.example.com/webhooks/telegram
  chat_id: -100123456789

slack:
  enabled: true
  bot_token: "xoxb-..."          # APP_SLACK_BOT_TOKEN, not the file
  signing_secret: "..."          # APP_SLACK_SIGNING_SECRET, not the file
  channel_id: "C0123456789"
  webhook_path: /webhooks/slack/interactions
```

`internal/config/config.go`:

```go
type TelegramConfig struct {
    Enabled       bool   `koanf:"enabled"`
    BotToken      string `koanf:"bot_token"`
    WebhookSecret string `koanf:"webhook_secret"`
    WebhookPath   string `koanf:"webhook_path"`
    WebhookURL    string `koanf:"webhook_url"`
    ChatID        int64  `koanf:"chat_id"`
}

type SlackConfig struct {
    Enabled       bool   `koanf:"enabled"`
    BotToken      string `koanf:"bot_token"`
    SigningSecret string `koanf:"signing_secret"`
    ChannelID     string `koanf:"channel_id"`
    WebhookPath   string `koanf:"webhook_path"`
}
```

Validation (`internal/config/load.go`, after struct-tag validation passes):
each config's required fields (`bot_token`, `chat_id`/`channel_id`, etc.) are
only enforced when that platform's `enabled` is `true` — implemented as a
manual post-validation check rather than a `validator` struct tag, since
conditional-required-if-sibling-field-true crosses two different structs.
A second manual check fails startup if both `telegram.enabled` and
`slack.enabled` are `false`.

### 2. `internal/slackbot` (new package)

Mirrors `internal/telegrambot`'s shape:

```go
type Block any // Slack Block Kit element; kept opaque here, built in event_*.go

type Client interface {
    // PostMessage sends a message to channel and returns its ts (timestamp ID).
    PostMessage(ctx context.Context, channel, text string, blocks []Block) (ts string, err error)
    // UpdateMessage edits an existing message in place. Pass blocks=nil to
    // strip any buttons — Slack has no separate "clear markup" call; chat.update
    // with omitted blocks does both jobs (see docs/specs research above).
    UpdateMessage(ctx context.Context, channel, ts, text string, blocks []Block) error
}
```

Real implementation (`slack_client.go`) uses `github.com/slack-go/slack`
(the de facto standard Go Slack SDK — same "thin adapter over an SDK" pattern
as `telegrambot`/`githubapp`). Test double `slackbottest.FakeClient` records
calls the same way `telegrambottest.FakeClient` does today, for use in
`internal/httpserver`'s `testServer` harness and `internal/notify`/`internal/retry`
unit tests.

### 3. Notify handlers — parallel templates, independent sends

Each `internal/notify/event_*.go` handler gains a `slack slackbot.Client`
field (nil-able — nil means Slack is disabled, and the handler skips that
half). Two `templateLoader` instances are constructed in `notify.New`, one
per platform, both reading the same `templates/` directory but by different
file suffix:

- `push.tmpl` → Telegram (HTML parse mode, unchanged)
- `push.slack.tmpl` → Slack (`mrkdwn`)

Each `Handle()` renders both (skipping either that returns empty output,
same "empty means skip this platform" rule as today) and sends independently
— a delivery failure on one platform is logged, not fatal, and does not
block the other. This mirrors the existing "log and skip" pattern used for
template render errors.

New default templates needed for every existing event type: `push.slack.tmpl`,
`pull_request.slack.tmpl`, `create.slack.tmpl`, `issue_comment.slack.tmpl`,
`pull_request_review.slack.tmpl`, `pull_request_review_comment.slack.tmpl`,
`workflow_run.slack.tmpl`, `release.slack.tmpl`. Slack `mrkdwn` syntax differs
from Telegram HTML (e.g. `*bold*` not `<b>`, `<https://url|text>` not
`<a href>`), so these are hand-written per event, not derived automatically.

### 4. Retry — the one place needing shared cross-platform state

New migration (next sequential number after the current latest in
`internal/database/migrations/`):

```sql
-- 000004_add_cicd_retry_targets.up.sql
CREATE TABLE cicd_retry_targets (
    retry_id    INTEGER NOT NULL REFERENCES cicd_retries(id),
    platform    TEXT NOT NULL,       -- "telegram" | "slack"
    chat_ref    TEXT NOT NULL,       -- Telegram chat_id (as string) or Slack channel_id
    message_ref TEXT NOT NULL,       -- Telegram message_id (as string) or Slack ts
    PRIMARY KEY (retry_id, platform)
);
```

```sql
-- 000004_add_cicd_retry_targets.down.sql
DROP TABLE cicd_retry_targets;
```

`cicd_retries` itself is unchanged (still the source of truth for
`pending`/`retried`/`expired` status and the retryable `run_id`/`repo`); only
the Telegram-specific `chat_id`/`message_id` addressing moves out into the
new per-platform table. `database.Repository` gains
`CreateRetryTarget`/`ListRetryTargets` methods.

`retry.Handler.NotifyFailure`: posts the failure message to every enabled
platform (rendering `workflow_run.tmpl`/`workflow_run.slack.tmpl` as
appropriate), inserts one `cicd_retries` row plus one `cicd_retry_targets`
row per platform actually posted to.

`retry.Handler.HandleCallback`: unchanged trigger logic (look up the retry
row by ID, verify `pending`, call `github.RerunFailedJobs` once) — the
change is in the response fan-out step: instead of editing one Telegram
message, it loads all `cicd_retry_targets` rows for that `retry_id` and calls
the matching platform's edit/strip-buttons method for each. Same fan-out
applies to `retry.RunSweep`'s expiry path.

### 5. New endpoint: `/webhooks/slack/interactions`

`internal/httpserver/slack_webhook.go` (new file, parallel to
`telegram_webhook.go`):

- Verify `X-Slack-Signature` / `X-Slack-Request-Timestamp` per Slack's v0
  HMAC-SHA256 scheme (`v0:{timestamp}:{raw body}`, hex digest, compared via
  `crypto/subtle.ConstantTimeCompare` — same constant-time pattern already
  used for Telegram's secret-token check), rejecting requests with a stale
  timestamp (replay-window check, e.g. reject if `|now - timestamp| > 5m`).
- Body is `application/x-www-form-urlencoded` with a `payload` field
  containing JSON; decode into a minimal `block_actions` struct (action
  `value` carries the retry ID, mirroring Telegram's `callback_data`
  encoding — reuse `telegrambot.EncodeCallback`'s `hedwig:<feature>:<action>:<payload>`
  format as the button's `value` string for consistency, decoded with the
  existing `telegrambot.DecodeCallback`).
- Routes into the same `retry.Handler.HandleCallback` used by the Telegram
  path — no separate retry-trigger logic duplicated.

`cmd/bot/main.go` wires the new route only when `cfg.Slack.Enabled`.

## Slack app setup (operational, not code)

Documented in `docs/deployments/kubernetes.md` and `README.md`:

1. Create a Slack app (from manifest or manually) in the target workspace.
2. Bot Token Scopes: `chat:write` (post/update messages in the configured
   channel; add `chat:write.public` only if the bot shouldn't be invited to
   the channel first).
3. Enable **Interactivity & Shortcuts**, set Request URL to
   `https://<host>/webhooks/slack/interactions`.
4. Install the app to the workspace, copy the Bot User OAuth Token
   (`xoxb-...`) into `APP_SLACK_BOT_TOKEN`, and the Signing Secret into
   `APP_SLACK_SIGNING_SECRET`.
5. Invite the bot to the target channel (`/invite @hedwig`) and copy its
   channel ID into `slack.channel_id`.

## Testing

- `internal/slackbot`: unit tests for the real client's request-building
  (same shape as `telegrambot`'s), plus a `slackbottest.FakeClient`.
- `internal/notify`: existing `TestXHandler*` tests extended with a Slack
  client + Slack template assertions, following the existing
  `mustLoader`/`unmarshalEvent` helpers.
- `internal/retry`: extend `TestHandleCallback*` to cover multi-target rows
  — tapping either platform's callback clears both targets' buttons.
- `internal/httpserver`: new `slack_webhook_test.go` mirroring
  `telegram_webhook_test.go` (signature validation, replay rejection,
  malformed payload, routes to `retry.Handler`).
- `go build ./...`, `go vet ./...`, `go test -race ./...` stay clean
  throughout, per existing project convention.

## Out of scope (explicitly deferred)

- Unified `Notifier`/`Target` interface across platforms — revisit only if
  a third platform is requested.
- Per-repo/per-event Slack channel routing — single fixed channel for now.
- Slack Socket Mode — the existing public-HTTPS-webhook pattern (used for
  GitHub and Telegram already) is reused instead of adding a second,
  WebSocket-based connection model.
