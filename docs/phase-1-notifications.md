# Phase 1: Notifications & CI/CD Retry Bot

**Status: shipped** (this is what's currently implemented on `main`). See
[`docs/phase-2-pr-creation.md`](phase-2-pr-creation.md) for the planned
PR-creation phase.

## 1. Overview

A service connecting GitHub and Telegram: relays GitHub repository events
(push, PR opened/closed, CI/CD status) to Telegram, with a retry action for
failed CI/CD runs. Uses one GitHub App identity, one Telegram bot, and one
persistence layer.

This document covers the notifications + CI/CD retry bot only.

## 2. Goals

- Reduce time-to-notice for CI/CD failures and PR activity by pushing them into Telegram instead of requiring someone to check GitHub.
- Allow retrying a failed CI/CD run directly from Telegram, without opening GitHub.
- Keep scope proportional to actual usage, so engineering effort matches problems that actually occur at current scale.

## 3. Non-Goals

- Full parity with GitHub's UI (e.g. full-workflow rerun, single-job rerun) — intentionally out of scope, see Section 6.
- Multi-replica horizontal scaling — out of scope at current usage volume (see Section 8).
- Supporting non-GitHub Git hosts.
- Supporting non-Telegram messaging platforms.

## 4. User Stories

- As an engineer, I want to receive a Telegram message when someone pushes to a repository, so I stay aware of activity without polling GitHub.
- As an engineer, I want to receive a Telegram message when a PR is opened or closed, so I know when review is needed or a PR has landed.
- As an engineer, I want to receive a Telegram message when a CI/CD run fails, with a button to retry the failed jobs, so I can recover from transient failures without leaving the chat.

## 5. Functional Requirements

### 5.1 GitHub Event Notifications

| Event | GitHub Webhook | Trigger Condition | Telegram Message Content |
|---|---|---|---|
| Push | `push` | any push | pusher, ref, commit summary |
| PR opened | `pull_request` | `action == "opened"` | PR title, author, source→target branch, link |
| Tag created | `create` | `ref_type == "tag"` | repo, tag name, pusher, link |
| Branch created | `create` | `ref_type == "branch"` | repo, branch name, pusher, link |
| PR closed | `pull_request` | `action == "closed"` | PR title, merged vs. closed-without-merge, link |
| PR comment | `issue_comment` | `action == "created"` and `issue.pull_request` is present | PR title, commenter, comment excerpt, link |
| PR review | `pull_request_review` | `action == "submitted"` | PR title, reviewer, review state (approved/changes requested/commented), link |
| PR review comment | `pull_request_review_comment` | `action == "created"` | PR title, commenter, file/line, comment excerpt, link |
| CI/CD started | `workflow_run` | `action == "requested"` | workflow name, repo, branch, link |
| CI/CD status | `workflow_run` | `action == "completed"` | workflow name, repo, conclusion (success/failure/cancelled), link; **retry button attached only on failure** |

All dynamic content embedded in these HTML-parse-mode messages (commit
messages, PR titles/bodies, comment excerpts, branch/tag names, usernames) is
HTML-escaped before being sent, so `<`/`>`/`&` in GitHub-supplied content can't
break message delivery or inject markup into the Telegram message.

### 5.2 CI/CD Retry Button

- **Single button only**: "Retry failed jobs" — calls `POST /repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs`.
- No full-workflow rerun button, no single-job rerun button (see Section 6 for rationale).
- On tap:
  1. Answer the callback query immediately (avoids stuck loading spinner).
  2. Call the GitHub rerun-failed-jobs API using the stored `run_id`. Since the sweep (5.2.1) removes the button before the 24-hour window closes, every tap reaching this step is within the valid window.
  3. On success: edit the message to remove the button and indicate "Retrying...".
  4. On failure (e.g. run still active, already running): edit the message to show a plain error including a link back to the GitHub Actions run, so the user can check status or retry manually from GitHub if needed. Keep the record `pending` so a later tap can still be attempted within the expiry window.

#### 5.2.1 Button Expiry

- Each retry button is valid for **24 hours** from the failure notification.
- **Expiry is enforced by a periodic background sweep** (e.g. every 30 minutes): it finds `pending` records older than 24 hours, strips their keyboards, and marks them `expired`.

### 5.3 Access Control

- Only Telegram users on a **statically configured allowlist of user IDs** (numeric, stable Telegram identifiers — not usernames, which can change) may invoke bot commands or tap inline keyboard buttons (the retry button).
- The allowlist lives in the same static configuration file as the repository list (Section 7.2), so no separate config mechanism is needed.
- Enforcement happens once, in a single shared check in `telegrambot/`, applied before any update is dispatched to `notify/` or `retry/` — not duplicated per feature.
- Requests from users not on the allowlist are **silently ignored**, so the bot doesn't confirm its own existence or behavior to an unrecognized user probing it.

## 6. Explicit Limitations (By Design)

This system is deliberately scoped down. Each item below is a conscious tradeoff to keep the system maintainable at current usage levels:

- **No full-workflow rerun option.** Retry-failed-jobs alone covers the large majority of real failures (transient/flaky steps). Full rerun can be done manually in GitHub if genuinely needed.
- **No single-job retry.** GitHub's API only allows re-running one job at a time, and doing so starts a new "attempt," which then blocks re-running any other still-failed jobs from the old attempt individually (`"Only jobs from the current attempt can be re-run"`). Avoiding this entirely sidesteps a real API quirk rather than working around it.
- **"Already running" conflicts are not pre-checked.** The system relies on GitHub only sending `workflow_run` `action: "completed"` after a run (including cleanup) has actually finished, which removes most of the race window. If a rerun call still fails because the run is unexpectedly active, the user sees a plain error message rather than the system polling GitHub proactively to avoid it.
- **Retry buttons expire after 24 hours**, enforced by a background sweep that strips unused buttons after that window (see 5.2.1).
- **Rate limits (GitHub API and Telegram Bot API) are an accepted risk, not actively handled.** At current usage volume, GitHub's 5,000 requests/hour authenticated limit and Telegram's roughly 1 message/second per chat limit are unlikely to be hit. No retry/backoff logic is built for this; if it becomes a real problem at higher volume, it can be addressed then.
- **Duplicate webhook deliveries are actively deduplicated, not just accepted.** GitHub retries a webhook delivery if the endpoint times out or returns a non-2xx response, which could otherwise cause a duplicate Telegram notification for the same event. See Section 7.3 for the storage-backed dedupe mechanism.
- **A dispatch failure un-does the dedupe record and returns a non-2xx response**, so a transient failure (e.g. a Telegram API blip) causes GitHub to retry the delivery instead of the notification being silently lost.

## 7. Technical Design

### 7.1 Stack

| Concern | Choice |
|---|---|
| Language | Go |
| Configuration | `knadh/koanf` (YAML/TOML file + env var layering) + `go-playground/validator` (struct validation at startup) |
| GitHub REST client | `google/go-github` |
| GitHub App auth | `jferrl/go-githubauth` |
| Webhook parsing | `github.ValidatePayload` + `github.ParseWebHook` + typed event structs |
| Telegram client | `go-telegram/bot` |
| Persistence | SQLite (WAL mode) via `modernc.org/sqlite` (pure Go implementation, no CGO/C toolchain dependency) |
| Deployment | Container on EKS; separate pipeline from existing infra repos, built via GitHub Actions, images pushed to GitHub Container Registry (GHCR) |

### 7.2 Configuration & Validation

Configuration is layered from two sources, loaded via `koanf`:

1. **File (YAML or TOML)** — non-secret structural config: repository allowlist, listen port, GitHub App private key path.
2. **Environment variables** — secrets: Telegram bot token, GitHub webhook secret, Telegram webhook secret token. Env vars override/fill in on top of the file, using a configurable prefix (e.g. `APP_`).

The **GitHub App private key** is provided as a **file path** in config (e.g. `github.private_key_path`, itself overridable via file or env), pointing to a Kubernetes Secret mounted as a volume at startup. This keeps the private key out of `os.Environ()` and process/crash dumps.

**Validation happens once, at startup, via `go-playground/validator`** using struct tags (e.g. `validate:"required"`, `validate:"required,min=1,dive,required"` for the repo list, `validate:"required,file"` for the private key path). The service fails fast — logging the validation error and exiting — rather than starting with incomplete config and failing later on the first webhook or Telegram update. This also applies to the private key content itself: presence of the file is checked by `validator`, but the file's contents are separately parsed as an RSA private key at startup (before the private key is handed to `jferrl/go-githubauth`), so a malformed PEM is also caught immediately rather than surfacing on the first GitHub App auth attempt.

### 7.3 Data Model (SQLite)

Both tables below hold transient state only — rows are not kept as permanent history. Once a record reaches a terminal status (`retried`/`expired` for `cicd_retries`, or simply past a short retention window for `webhook_deliveries`), it can be deleted rather than retained indefinitely. This can be done inline when the terminal state is reached, or swept periodically alongside the existing expiry sweep (see 5.2.1) — either approach keeps the SQLite file from growing unbounded.

```sql
CREATE TABLE webhook_deliveries (
    delivery_id TEXT PRIMARY KEY, -- GitHub's X-GitHub-Delivery header, used to dedupe retried deliveries
    received_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE cicd_retries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id INTEGER NOT NULL,
    message_id INTEGER NOT NULL,
    run_id INTEGER NOT NULL,
    repo TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', -- pending | retried | expired
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Idempotency**: before dispatching any parsed GitHub webhook event to `notify/`, the handler attempts to insert the delivery's `X-GitHub-Delivery` header value into `webhook_deliveries`. Relying on the primary key's uniqueness constraint to reject a duplicate insert (rather than a separate check-then-insert) avoids a race between checking and recording. If the insert fails as a duplicate, the handler responds `200 OK` (so GitHub stops retrying) but skips processing, since the event has already been handled. If dispatching the event to `notify/` fails, the delivery record is deleted and the handler responds with a non-2xx status, so GitHub retries the delivery instead of the notification being silently lost.

### 7.4 Required GitHub App Permissions & Webhook Subscriptions

| Permission | Level | Needed For |
|---|---|---|
| Contents | Read | `push` and `create` (tag/branch) events |
| Pull requests | Read | `pull_request`, `pull_request_review`, `pull_request_review_comment` events |
| Issues | Read | `issue_comment` events (fires for both issue and PR comments; PR comments are distinguished by the presence of `issue.pull_request` in the payload) |
| Actions | Read & Write | `workflow_run` events (read) + rerun-failed-jobs (write) |

Webhook subscriptions to enable on the GitHub App registration: `push`, `create`, `pull_request`, `issue_comment`, `pull_request_review`, `pull_request_review_comment`, `workflow_run`.

(Phase 2 — see [`docs/phase-2-pr-creation.md`](phase-2-pr-creation.md) — will need Pull Requests write access again once implemented.)

### 7.5 Endpoints Exposed

| Endpoint | Purpose | Auth |
|---|---|---|
| `/webhooks/github` | Receives GitHub webhook deliveries | HMAC signature (`X-Hub-Signature-256`) validated via `github.ValidatePayload` |
| `/webhooks/telegram` | Receives Telegram bot updates | `X-Telegram-Bot-Api-Secret-Token` header validated (constant-time comparison) |

### 7.6 Packages

Brief map of each Go package to the feature it supports, with the directory structure it lives in:

```
hedwig/
├── cmd/
│   └── bot/
│       └── main.go              # entry point: load config, initialize clients, start server
│
├── internal/
│   ├── config/                  # loads and validates configuration (Section 7.2)
│   ├── githubapp/                # GitHub App client: authentication, webhook parsing, API calls
│   ├── telegrambot/               # Telegram bot client: message sending, inline keyboards, callbacks
│   │
│   ├── notify/                   # Feature: GitHub → Telegram notifications (Section 5.1)
│   ├── retry/                     # Feature: CI/CD retry button and its 24-hour expiry (Section 5.2)
│   │
│   ├── storage/                  # SQLite access (Section 7.3): tables, queries, migrations
│   ├── httpserver/                # HTTP routes for the two webhook endpoints (Section 7.5)
│   └── logging/                   # structured JSON logger with request correlation ID
│
├── config.example.yaml            # sample non-secret configuration file
├── Dockerfile
└── go.mod
```

| Package | Short Explanation |
|---|---|
| `config/` | Reads the YAML/TOML file and environment variables into one configuration object, and checks it's valid before the service starts |
| `githubapp/` | Wraps calls to GitHub: authenticating as the GitHub App, reading incoming webhook payloads, and calling the GitHub Actions/Pull Request Application Programming Interface (API) |
| `telegrambot/` | Wraps calls to Telegram: sending messages, building inline keyboards (buttons attached to a message), and handling button taps |
| `notify/` | Turns a GitHub webhook event into a Telegram message (Section 5.1) |
| `retry/` | Handles the "Retry failed jobs" button and its 24-hour expiry (Section 5.2) |
| `storage/` | Reads and writes the SQLite tables (`webhook_deliveries`, `cicd_retries`) |
| `httpserver/` | Exposes the two Hypertext Transfer Protocol (HTTP) endpoints that receive webhook deliveries from GitHub and Telegram |
| `logging/` | Writes structured JavaScript Object Notation (JSON) logs with a request ID for tracing a single event across the system |

### 7.7 Coding Patterns per Package

| Package | Pattern | Why |
|---|---|---|
| `notify/` | Strategy pattern + registry (a lookup table mapping each event type to its handler) | Every GitHub webhook event type follows the same process (parse → build message → send), but differs in content and message template; one handler implementation per event type keeps this consistent and easy to extend |
| `retry/` | Explicit state transitions (a small, hand-written state machine — a Finite State Machine, or FSM, is a model where something can only be in one defined state at a time and moves between states through defined rules) | Only two real states (pending, retried/expired), but transitions should still go through one controlled function so an invalid state change (e.g. an expired button being retried) is impossible by construction |
| `githubapp/`, `telegrambot/` | Adapter pattern (a thin wrapper exposing only the specific operations needed, behind a small interface — a contract describing what methods a type must have, without dictating how) | Keeps GitHub API and Telegram Bot API calls swappable for test doubles (fake implementations used in automated tests instead of the real service) |
| `storage/` | Repository pattern (an interface that hides how data is actually stored, e.g. behind SQLite, so calling code only deals with simple read/write operations) | Lets the CI/CD retry sweep logic be tested without a real SQLite file |
| `config/` | Plain loader function (no builder or fluent options pattern) | Configuration is loaded once at startup and never reconfigured at runtime, so a simpler one-shot function is enough |
| `main.go` (entry point) | Manual constructor injection (passing dependencies directly into constructor functions, instead of using a Dependency Injection, or DI, framework that wires them automatically) | Keeps the full dependency graph visible in one file, which matters more at this project's size than the convenience a DI framework would add |

The common thread: wherever multiple things share the same overall process but differ in content (event types in `notify/`), use the Strategy pattern with a registry. Wherever there's an ordered progression with state that must persist (retry buttons), make the states and transitions explicit rather than implicit in scattered conditional checks.

### 7.8 Application Packaging

**Build**: a multi-stage Dockerfile — a first stage compiles the Go binary (`go build`), a second, minimal `distroless` stage copies only the compiled binary in, keeping the final container image small and free of the Go toolchain.

```dockerfile
# Stage 1: build
FROM golang:1.25 AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -o bot ./cmd/bot   # pure Go, no C toolchain needed

# Stage 2: run
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/bot /usr/local/bin/bot
ENTRYPOINT ["/usr/local/bin/bot"]
```

Since persistence uses `modernc.org/sqlite` (Section 7.1), the build runs fully with `CGO_ENABLED=0`, producing a fully static binary suited to a minimal `distroless/static` runtime stage: no C library, no shell, minimal attack surface.

**Image build and publish**: handled by a GitHub Actions workflow (separate from the existing infrastructure repos' pipelines, per Section 7.1), which builds the image and pushes it to the GitHub Container Registry (GHCR) on merge to the main branch, tagged with both the commit Secure Hash Algorithm (SHA) and a semantic version or `latest` tag as appropriate.

**Kubernetes deployment shape**:

| Resource | Purpose |
|---|---|
| Deployment | `replicas: 1` (Section 8), references the GHCR image, mounts the ConfigMap and Secret below |
| ConfigMap | The non-secret YAML/TOML configuration file (Section 7.2), mounted as a file into the container |
| Secret | Environment variables for tokens/secrets (Telegram bot token, GitHub/Telegram webhook secrets), plus the GitHub App private key mounted as a file (Section 7.2) |
| PersistentVolumeClaim (PVC) | Backs the SQLite database file so it survives pod restarts (Section 8) |
| Service | Exposes the container's HTTP port inside the cluster |
| Ingress (or Istio Gateway/VirtualService, matching existing cluster conventions) | Routes external GitHub and Telegram webhook deliveries to the Service |

**Health checks**: a liveness probe (Section 8) against a simple `/healthz` endpoint in `httpserver/`, so a hung process gets restarted even if it hasn't crashed outright. A readiness probe on the same or a similar endpoint prevents traffic being routed to the pod before the SQLite connection and configuration are fully initialized.

## 8. Operational Notes

- **Single replica (`replicas: 1`)**, backed by an EBS-backed PVC (ReadWriteOnce is sufficient for one writer) so the SQLite file survives pod restarts. This is a non-critical internal tool, so brief downtime on crash is acceptable.
- SQLite in **WAL mode** to allow concurrent reads/writes within the single process.
- **Self-healing on crash**: Kubernetes' default restart policy brings the pod back automatically; since all retry state lives in the PVC-backed SQLite file rather than in-memory, a restarted pod resumes from where it left off (pending retries) rather than losing state. A liveness probe should be configured so a hung (not just crashed) process also gets restarted rather than sitting unresponsive.
- Logs written to **stdout as structured JSON**, including a `request_id` (or equivalent correlation ID) per related request/process/event, so a webhook delivery, its downstream Telegram message, and any retry callback can be traced through the same ID. Log aggregation (e.g. shipping stdout to the existing Loki stack) is an infra-level concern outside this service's own responsibility.
- A pod restart mid-flow (pending retry) should not corrupt state, since it lives in SQLite rather than in-memory.
