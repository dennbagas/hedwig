# Hedwig

A service that bridges GitHub webhooks to Telegram notifications, with a CI/CD retry button for failed workflow runs.

## Features

- Relays GitHub events (push, PR, CI/CD status) to a Telegram chat
- Retry button on failed CI/CD runs — one tap re-triggers the workflow
- Deduplicates GitHub webhook retries via delivery ID tracking
- Single GitHub App identity + single Telegram bot

See the [Phase 1 PRD](docs/phase-1-notifications.md) for full requirements and design.
A [Phase 2 PR-creation feature](docs/phase-2-pr-creation.md) is planned but not yet implemented.

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
docker run -v /path/to/config.yaml:/app/config.yaml hedwig
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
