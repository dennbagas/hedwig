# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build the bot binary
go build -o bot ./cmd/bot

# Run the bot (requires config.yaml)
go run ./cmd/bot -config config.yaml

# Run migrations / initialize the SQLite database
go run ./cmd/migrate -db hedwig-dev.db

# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/notify/...

# Run a single test
go test ./internal/retry/... -run TestHandleCallback

# Lint
go vet ./...

# Build Docker image
docker build -t hedwig .
```

## Configuration

Configuration is layered: YAML file first, then `APP_`-prefixed environment variables override on top. The mapping uses dot-notation → underscore: `github.webhook_secret` → `APP_GITHUB_WEBHOOK_SECRET`. Uses the `koanf` library; struct fields are tagged with `koanf:`.

Copy `config.example.yaml` to `config.yaml` to get started. Secrets (`bot_token`, `webhook_secret`, `github.webhook_secret`) are intended to be set via env vars, not in the file. The GitHub App private key is a **file path** in config (never inline), parsed and validated at startup.

Validation runs at startup via `go-playground/validator` struct tags — the service exits immediately on any missing required field or unparseable private key rather than failing on first use.

## Architecture

Hedwig bridges GitHub webhooks → Telegram notifications, with two additional features: a CI/CD retry button and a Telegram-driven PR creation flow. A single GitHub App identity and a single Telegram bot serve all three features.

### Request flow

```
GitHub webhook → POST /webhooks/github
    → httpserver.handleGitHubWebhook
        → githubapp.ValidateWebhook (HMAC check)
        → storage.RecordDelivery (deduplication via unique constraint on X-GitHub-Delivery)
        → githubapp.ParseWebhook → typed event struct
        → notify.Dispatcher.Dispatch → per-event EventHandler
            (workflow_run failure → retry.Handler.NotifyFailure → stores CICDRetry, attaches button)

Telegram update → POST /webhooks/telegram
    → httpserver.handleTelegramWebhook
        → secret token validation
        → allowedUserIDs check (silently drops unknown users)
        → /newpr command   → prcreate.Handler.HandleCommand
        → callback query   → retry.Handler.HandleCallback | prcreate.Handler.HandleCallback
        → plain text       → prcreate.Handler.HandleTextMessage (fed to active session step)

Background goroutines (30-minute interval):
    → retry.RunSweep  — expires pending CICDRetry rows older than 24h, strips buttons
    → prcreate.RunSweep — expires in-progress PRSession rows older than 24h, strips keyboards
```

### Key design patterns

| Package | Pattern |
|---|---|
| `notify/` | Strategy + registry: `Dispatcher` maps event type strings to `EventHandler` implementations. Add a new event by implementing `EventHandler` and calling `d.Register(...)` in `register.go`. |
| `prcreate/` | FSM with transition table: each step is explicit (`select_repo`, `enter_title`, `enter_message`, `confirm`) and persisted in SQLite so pod restarts don't lose state. Terminal states are `done` and `cancelled`. |
| `retry/` | Explicit state machine: `pending → retried/expired`. State transitions go through `storage.UpdateRetryStatus`. |
| `githubapp/`, `telegrambot/` | Adapter/interface: `githubapp.Client` and `telegrambot.Client` are thin interfaces over the real SDKs, making them swappable for test doubles. |
| `storage/` | Repository pattern: `storage.Repository` interface hides SQLite; inject it to test feature logic without a real DB file. |
| `main.go` | Manual constructor injection — the full dependency graph is wired in one place, no DI framework. |

Logging uses `go.uber.org/zap`. A request-scoped logger is threaded through context via `logging.FromContext(ctx)` / the middleware in `httpserver/middleware.go`.

### Persistence (SQLite, WAL mode)

Three tables, all transient (no permanent history):

- `webhook_deliveries` — deduplicates GitHub webhook retries; cleaned periodically.
- `cicd_retries` — one row per pending retry button; statuses: `pending → retried | expired`.
- `pr_sessions` — one row per active PR creation flow; statuses: `in_progress → completed | cancelled | expired`. Primary key is `(chat_id, message_id)`.

Deduplication relies on the primary key unique constraint (not check-then-insert) to avoid a race condition. If `RecordDelivery` returns `isDuplicate=true`, the handler returns `200 OK` to stop GitHub retrying but skips processing.

### Adding a new GitHub event handler

1. Implement `notify.EventHandler` (`Handle(ctx, event) error`) in `internal/notify/`.
2. Register it in `internal/notify/register.go` via `d.Register("event_type", &yourHandler{...})`.
3. The event type string must match what `github.ParseWebHook` returns (e.g. `"push"`, `"workflow_run"`).

Currently registered event types: `push`, `pull_request`, `create`, `issue_comment`, `pull_request_review`, `pull_request_review_comment`, `workflow_run`.

### Callback data encoding

`telegrambot.EncodeCallback(feature, action, payload)` produces the callback data string `hedwig:<feature>:<action>:<payload>` stored on inline buttons. The Telegram webhook handler decodes this to route to the right handler. The `retry` package uses `feature="retry"`, `action="trigger"`; the `prcreate` package uses `feature="pr"` with actions `repo`, `confirm`, `skip`, `cancel`.
