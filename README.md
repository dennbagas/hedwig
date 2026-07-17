# Hedwig

A service that bridges GitHub webhooks to Telegram notifications, with a CI/CD retry button for failed workflow runs.

## Features

- Relays GitHub events (push, PR, releases, CI/CD status) to a Telegram chat
- Fully configurable notification templates — one Go `text/template` file per event type, hot-swappable via Kubernetes ConfigMap
- Retry button on failed CI/CD runs — one tap re-triggers the workflow
- Deduplicates GitHub webhook retries via delivery ID tracking
- Single GitHub App identity + single Telegram bot

## Supported Events

| Event | Webhook | Default behavior |
|---|---|---|
| Push | `push` | Always notifies |
| Pull request opened | `pull_request` | Notifies on `opened` |
| Pull request merged | `pull_request` | Notifies on `closed` + merged |
| PR comment | `issue_comment` | Notifies on `created` (PR comments only) |
| PR review | `pull_request_review` | Notifies on `submitted` |
| PR review comment | `pull_request_review_comment` | Notifies on `created` |
| Branch created | `create` | Notifies on `branch` type |
| Release published | `release` | Notifies on `published` |
| CI/CD completed | `workflow_run` | Notifies on `completed`; attaches retry button on failure |

## Prerequisites

- Go 1.26+
- A registered [GitHub App](https://docs.github.com/en/apps/creating-github-apps) with webhook events enabled
- A Telegram bot (via [@BotFather](https://t.me/BotFather)) with webhook configured
- A public HTTPS endpoint for receiving webhooks

## Setup

```bash
# Clone and install dependencies
git clone https://github.com/dennbagas/hedwig
cd hedwig
go mod download

# Copy and fill in the config
cp config.example.yaml config.yaml
# Edit config.yaml — secrets can also be set via APP_* env vars (see config.example.yaml)
```

## Running

```bash
# Hot-reload dev server (installs air if missing)
make dev

# Build binary
make build

# Run binary directly
./tmp/bot -config config.yaml

# Run tests
make test
```

## Docker

```bash
docker build -t hedwig .
docker run \
  -v /path/to/config.yaml:/app/config.yaml \
  -v /path/to/templates:/etc/hedwig/templates \
  hedwig
```

## Configuration

All config is in `config.yaml` (YAML) with `APP_`-prefixed env var overrides.
Secrets should be set via env vars rather than the file:

| Env var | Config field |
|---|---|
| `APP_GITHUB_WEBHOOK_SECRET` | `github.webhook_secret` |
| `APP_TELEGRAM_BOT_TOKEN` | `telegram.bot_token` |
| `APP_TELEGRAM_WEBHOOK_SECRET` | `telegram.webhook_secret` |

See `config.example.yaml` for all available fields.

## Notification Templates

Each event type has a corresponding `.tmpl` file in the configured templates directory (default: `/etc/hedwig/templates`). Templates are standard Go `text/template` files — they receive a typed context struct and produce the Telegram message text.

Default templates live in [`templates/`](templates/) at the project root. Copy them as a starting point:

```bash
cp templates/ /etc/hedwig/templates/
```

A template that produces empty output silently skips the notification, which is how action filtering works:

```
{{- if eq .Action "opened" -}}
📢 New Pull Request: {{.Title}}
{{- end -}}
```

All string fields in context structs are pre-HTML-escaped, so templates are safe to use with Telegram's `HTML` parse mode.
