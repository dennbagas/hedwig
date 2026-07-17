# CLAUDE.md

Guidance for Claude Code when working in this repository.

## Commands

```bash
make dev          # hot-reload dev server via air (installs air if missing)
make build        # build binary to ./tmp/bot
make test         # go test -race ./...

go test ./internal/notify/...              # single package
go test ./internal/retry/... -run TestHandleCallback  # single test
go vet ./...
docker build -t hedwig .
```

## Conventions

- Spec/design docs go in `./docs/specs/` (not `docs/superpowers/specs/` — override the superpowers brainstorming skill default).

## Architecture

Hedwig bridges GitHub webhooks → Telegram notifications. A single GitHub App and a single Telegram bot serve all features.

### Request flow

```
GitHub webhook → POST /webhooks/github
    → httpserver.handleGitHubWebhook
        → githubapp.ValidateWebhook        (HMAC signature check)
        → database.RecordDelivery          (dedup via unique constraint on X-GitHub-Delivery)
        → githubapp.ParseWebhook           (typed event struct)
        → notify.Dispatcher.Dispatch       (per-event EventHandler)
            workflow_run failure → retry.Handler.NotifyFailure
                → stores CICDRetry row, attaches inline retry button
        → on dispatch failure: database.DeleteDelivery + non-2xx, so GitHub retries

Telegram update → POST /webhooks/telegram
    → httpserver.handleTelegramWebhook
        → secret token validation          (constant-time compare)
        → allowedUserIDs check             (silently drops unknown users)
        → callback query → retry.Handler.HandleCallback

Background goroutine (30-min interval):
    → retry.RunSweep — expires CICDRetry rows older than 24h, strips buttons
```

### Package responsibilities

| Package | Role |
|---|---|
| `cmd/bot` | Wires the full dependency graph via manual constructor injection. No DI framework. |
| `internal/config` | Loads YAML + env vars via koanf, validates with go-playground/validator. Exits on first error. |
| `internal/githubapp` | Thin interface over go-github v88. `Client` interface keeps httpserver decoupled from the SDK. |
| `internal/telegrambot` | Thin interface over go-telegram/bot. Same pattern as githubapp. |
| `internal/notify` | Strategy + registry: `Dispatcher` maps event-type strings to `EventHandler` implementations. Each event type lives in its own `event_*.go` file alongside its context struct. Templates are loaded at startup from disk via `templateLoader`. |
| `internal/retry` | Explicit state machine: `pending → retried \| expired`. |
| `internal/database` | Repository pattern: `Repository` interface over SQLite (modernc, CGO-free). Migrations run at startup via golang-migrate with embedded SQL files under `migrations/`. |
| `internal/httpserver` | HTTP routing, middleware (request ID, logging), webhook handlers. |
| `internal/logging` | zerolog logger; request-scoped via `logging.FromContext(ctx)`. |

### Key design patterns

- **Adapters**: `githubapp.Client` and `telegrambot.Client` are interfaces over real SDKs — swap with test doubles (`githubapptest.FakeClient`, `telegrambottest.FakeClient`) in tests.
- **Deduplication**: `RecordDelivery` relies on a PRIMARY KEY unique constraint, not check-then-insert, to be race-safe.
- **Error propagation**: non-2xx on dispatch failure causes GitHub to retry; the delivery record is deleted first so the retry isn't skipped as a duplicate.
- **Notification templates**: each event handler calls `templateLoader.render(eventType, ctx)`. Empty output silently skips the notification — templates use `{{if eq .Action "opened"}}...{{end}}` to filter actions. All string fields in context structs are pre-HTML-escaped.

### Persistence (SQLite, WAL mode)

Two tables, both transient:

- `webhook_deliveries` — deduplicates GitHub webhook retries.
- `cicd_retries` — one row per pending retry button; statuses: `pending → retried | expired`.

Migrations live in `internal/database/migrations/` as numbered SQL files and are embedded into the binary at build time via `//go:embed`. They run automatically when `database.Open()` is called at startup — no separate migration step needed.

**How it works:** golang-migrate tracks applied versions in a `schema_migrations` table it manages. On startup it applies any unapplied `.up.sql` files in order, then returns. `ErrNoChange` (already up to date) is silently ignored.

**Adding a migration:**
Always create both files — a migration without a down file will be rejected by golang-migrate.
1. Create two files in `internal/database/migrations/`:
   - `000002_your_description.up.sql` — forward changes
   - `000002_your_description.down.sql` — rollback (DROP / reverse of up)
2. Use the next sequential number — never reuse or skip numbers.
3. Each file must contain only plain SQL; no explicit `BEGIN`/`COMMIT` (golang-migrate wraps each migration in an implicit transaction).
4. Rebuild and restart — the migration runs automatically on next startup.

**Do not use CHECK constraints** to enforce column values (e.g. valid status strings). SQLite has no `ALTER TABLE ... DROP CONSTRAINT`, so changing any CHECK constraint requires a full table rebuild migration. Enforce valid values in Go instead.

### Callback data encoding

`telegrambot.EncodeCallback(feature, action, payload)` → `hedwig:<feature>:<action>:<payload>`.
Stored on inline keyboard buttons; decoded by the Telegram webhook handler to route to the right handler.
The `retry` package uses `feature="retry"`, `action="trigger"`.

### Notification templates

Default templates live in `templates/` at the project root. In production they are mounted as a Kubernetes ConfigMap into the path set by `notifications.templates_dir`.

Each file is named `<event_type>.tmpl` and receives its typed context struct. The `templateLoader` logs a warning and skips the notification when no template file exists for an event type.

### Adding a new GitHub event handler

1. Create `internal/notify/event_<type>.go` with:
   - A `*Context` struct holding the template data (all string fields HTML-escaped).
   - A `*Handler` struct embedding `tg`, `chatID`, and `loader *templateLoader`.
   - A `Handle(ctx, event) error` method implementing `notify.EventHandler`.
2. Register it in `registerAll` in `internal/notify/dispatcher.go`.
3. Add a default template at `templates/<event_type>.tmpl`.
4. The event type string must match what `github.ParseWebHook` returns (e.g. `"push"`, `"workflow_run"`).

Currently registered: `push`, `pull_request`, `create`, `issue_comment`, `pull_request_review`, `pull_request_review_comment`, `workflow_run`, `release`.

## Configuration

Layered: YAML file first, then `APP_`-prefixed env vars override on top.
Mapping: replace the first `_` with `.` after stripping `APP_` and lowercasing.
Example: `APP_GITHUB_WEBHOOK_SECRET` → `github.webhook_secret`.

Secrets (`bot_token`, `webhook_secret`, `github.webhook_secret`) go via env vars, not the file.
The GitHub App private key is a **file path** in config — parsed and validated at startup.

`notifications.templates_dir` must point to a directory containing `.tmpl` files. All files are parsed at startup; a syntax error in any template aborts startup.
