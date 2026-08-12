# Hedwig

A service that bridges GitHub webhooks to Telegram and/or Slack notifications, with an optional CI/CD retry button for failed workflow runs.

## Features

- Relays GitHub events (push, PR, releases, CI/CD status) to Telegram and/or Slack — either channel can be enabled independently
- Fully configurable notification templates — one Go `text/template` file per event type per platform, hot-swappable via Kubernetes ConfigMap
- Optional retry button on failed CI/CD runs (`retry.enabled`, opt-in) — one tap re-triggers only the failed jobs; a run that can't be retried (e.g. already retried) shows a clear message instead of a dead button
- Retrying is idempotent: only one tap ever wins when the same retry has a live button on both platforms, or is double-tapped — GitHub's rerun endpoint is called at most once per retry, and only the winning tap's result is ever shown
- Deduplicates GitHub webhook retries via delivery ID tracking
- Single GitHub App identity + one bot per enabled channel

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
| CI/CD completed | `workflow_run` | Notifies on `completed`; attaches a retry button on failure if `retry.enabled: true` |

## Prerequisites

- Go 1.26+
- A registered GitHub App with webhook events enabled — see [docs/setup/github-app.md](docs/setup/github-app.md)
- At least one of: a Telegram bot — see [docs/setup/telegram.md](docs/setup/telegram.md) — or a Slack app — see [docs/setup/slack.md](docs/setup/slack.md)
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

Getting the actual values for `config.yaml` requires a one-time setup on GitHub's/Telegram's/Slack's side first — see [docs/setup/github-app.md](docs/setup/github-app.md), [docs/setup/telegram.md](docs/setup/telegram.md), and [docs/setup/slack.md](docs/setup/slack.md).

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

## Kubernetes

See [docs/kubernetes.md](docs/deployments/kubernetes.md) for a full guide covering:

- Creating secrets (GitHub App key, bot tokens, webhook secrets)
- ConfigMaps for `config.yaml` and notification templates
- Deployment, Service, and Ingress manifests
- Updating templates without a pod restart
- Upgrading the image

Pre-built images are published to `ghcr.io/dennbagas/hedwig` on every tagged release.

## Configuration

All config is in `config.yaml` (YAML) with `APP_`-prefixed env var overrides.
Secrets should be set via env vars rather than the file:

| Env var | Config field |
|---|---|
| `APP_GITHUB_WEBHOOK_SECRET` | `github.webhook_secret` |
| `APP_TELEGRAM_BOT_TOKEN` | `telegram.bot_token` |
| `APP_TELEGRAM_WEBHOOK_SECRET` | `telegram.webhook_secret` |
| `APP_SLACK_BOT_TOKEN` | `slack.bot_token` |
| `APP_SLACK_SIGNING_SECRET` | `slack.signing_secret` |

At least one of `telegram.enabled` / `slack.enabled` must be `true`; either can be enabled independently of the other.

`retry.enabled` (default `false`) controls whether `workflow_run` failures get a "Retry failed jobs" button at all — when disabled, failures are still notified, just as a plain message with no button and no calls to GitHub's rerun API.

See `config.example.yaml` for all available fields.

## Notification Templates

Each event type has a corresponding `.tmpl` file (Telegram) and `.slack.tmpl` file (Slack) in the configured templates directory (default: `/etc/hedwig/templates`) — a platform with no template file for an event simply skips that notification. Templates are standard Go `text/template` files — they receive a typed context struct and produce that platform's message text (HTML for Telegram, mrkdwn for Slack).

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

All string fields in context structs are pre-HTML-escaped, so Telegram templates are safe to use with Telegram's `HTML` parse mode. Slack templates use mrkdwn instead — no HTML escaping is needed there, but Slack's own mrkdwn special characters (`&`, `<`, `>`) in dynamic content are escaped by the code paths that need it (e.g. GitHub error messages surfaced by the retry button).
