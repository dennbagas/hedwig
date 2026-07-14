# Phase 2: PR Creation Bot

**Status: redesigned.** A first version (a Telegram wizard, "v1" below) was
built and briefly shipped on `development` (`internal/prcreate/`), then
reconsidered: real usage is dominated by deployments involving a fixed group
of applications with a repetitive title/description shape, which a
one-PR-at-a-time typed wizard handles poorly. It was removed from `main` and
replaced by a bulk, checklist-driven design ("v2" below), which is the
current plan for this phase — not yet implemented.

---

## v1 (superseded): Telegram wizard PR creation

This is the original phase 2 design: a single Telegram-driven, multi-step
flow to open one pull request between a chosen source and target branch, for
a chosen repository. Kept here for context on what existed and why it was
rethought — not the current plan.

### Goals

- Allow creating a pull request from Telegram via a guided, cancellable, multi-step interaction.

### User Stories

- As an engineer, I want to create a pull request by chatting with a Telegram bot (choosing repo, source branch, target branch), so I don't need to switch to GitHub or a terminal for routine PRs.
- As an engineer, I want to cancel the PR-creation flow at any step, so a wrong or abandoned start doesn't leave me stuck.

### Functional Requirements

Multi-step Telegram conversation:

1. User invokes the bot (command or button).
2. Bot presents repository choices from a statically configured list.
3. Bot asks the user to enter the PR title.
4. Bot asks the user to enter the PR message/details.
5. Bot presents a confirmation step summarizing repo, title, message, and the source/target branches (source and target branches are statically configured and identical across all repos, so no per-repo branch mapping is needed — no branch selection by the user).
6. On confirm: bot calls the GitHub API to create the PR, then edits the message to show the result (success + PR link, or error) and removes the keyboard.
7. **Cancel is available at every step** — cancelling clears the session and removes the keyboard immediately.
8. **Sessions expire after 24 hours** of inactivity, using the same expiry window and sweep mechanism as CI/CD retry buttons. An abandoned mid-flow session has its keyboard stripped and is marked `expired` by the same periodic sweep.

Each step's inline keyboard is replaced/edited in place rather than sent as a new message, so the chat doesn't fill up with stale keyboards from earlier steps.

### Access Control

Enforcement happens once, in a single shared check in `telegrambot/`, applied before any update is dispatched to `notify/`, `retry/`, or `prcreate/` — not duplicated per feature. Users may invoke bot commands or tap inline keyboard buttons (retry, PR-creation steps) only if on the allowlist.

### Data Model (SQLite)

```sql
CREATE TABLE pr_sessions (
    chat_id INTEGER NOT NULL,
    message_id INTEGER NOT NULL,
    step TEXT NOT NULL, -- select_repo | enter_title | enter_message | confirm | done | cancelled
    repo TEXT,
    pr_title TEXT,
    pr_message TEXT,
    status TEXT NOT NULL DEFAULT 'in_progress', -- in_progress | completed | cancelled | expired
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (chat_id, message_id)
);
```

One row per active PR creation flow, primary key `(chat_id, message_id)`,
transient (no permanent history) — same expiry/sweep pattern as
`cicd_retries` in phase 1.

### GitHub App Permissions

| Permission | Level | Needed For |
|---|---|---|
| Pull requests | Read & Write | reading PR events (phase 1) + PR creation (write, this feature) |

### Packages & Patterns

| Package | Pattern | Why |
|---|---|---|
| `prcreate/` | Finite State Machine (FSM) with a transition table, combined with the Strategy pattern per step | The PR creation flow has ordered steps, must support cancel from any step, and must survive a pod restart, so the current step and its data need to be explicit and stored, not just implicit in code flow |

### Known limitations that motivated the redesign

- One PR per conversation — no way to kick off a whole deployment's worth of
  PRs at once.
- Title and description are hand-typed every time, even though in practice
  they follow the same repetitive shape (a list of the constituent PR URLs
  going into a release) across a fixed set of applications.
- No connection to the team's actual deployment-tracking artifact (a Google
  Doc "deployment checklist"), so the wizard and the checklist were two
  disconnected sources of truth for the same release.

---

## v2 (current design): Bulk PR creation from a deployment checklist

### Context

In practice, deployments involve a fixed group of applications whose release
PRs all follow the same title/description shape — a reference list of the
individual PR URLs going into that release. Typing that out per app in the v1
wizard is repetitive.

The team already tracks each deployment in a human-edited Google Doc
("deployment checklist") listing, per application, the PR URL(s) being
shipped. The goal: forward that doc's link to the bot and have it open one
release PR per application automatically, using those PR URLs as the
description content — without turning the checklist into something that
looks/feels like a data file to the humans who maintain it.

Decisions made:
- **Format**: keep the checklist as a Google Doc, but require a **table**
  inside it (App | PR URL(s) column headers) — the Docs API exposes tables as
  structured rows/cells, so this parses as reliably as a spreadsheet while the
  doc still reads naturally, with prose allowed around the table.
- **Meaning of the PR URLs**: they are reference links to embed in the new
  release PR's description (changelog/traceability) — hedwig does not need to
  inspect or verify the state of those linked PRs.
- **Interface**: Telegram, via a new command (e.g. `/deploy <doc-url>`),
  reusing the existing bot identity and a confirm-before-create pattern
  (as v1 used).

Defaults assumed (flag if different behavior is wanted):
- **Partial failures**: continue best-effort through all rows and report a
  per-row result at the end, rather than aborting the whole batch on one
  failure — a broken row shouldn't block the rest of the deploy.
- **Unmatched app names**: rows whose "Application" text doesn't match any
  configured repo are skipped and called out in the confirmation summary
  (not silently dropped, not a hard error) — user can bail out via Cancel if
  something looks wrong before anything is created.

### New dependency & credentials

- Add `google.golang.org/api/docs/v1` (Go client for Docs API, read-only
  scope).
- New config section, following the existing GitHub App private-key pattern
  (file-path secret, validated at startup):
  ```yaml
  google:
    service_account_key_path: /etc/hedwig/google-sa.json
  ```
  The checklist doc must be shared (viewer) with the service account's email.
  This is an operational/doc step, not code, but should be called out in
  `README.md`/`config.example.yaml`.

### Config additions

- `internal/config/config.go`: new `GoogleConfig{ServiceAccountKeyPath
  string `validate:"required,file"`}` field on `Config`, validated the same
  way `GitHubConfig.PrivateKeyPath` is in `internal/config/load.go`.
- `RepoConfig` gets an optional `DisplayName` field used to match the
  checklist doc's "Application" column text case-insensitively; falls back to
  `Name` when unset.
- Add a `pr.deploy_title_template` / `pr.deploy_body_template` pair (Go
  `text/template` strings, e.g. `{{.App}}` / `{{.PRRefs}}`) to `PRConfig` so
  the release PR's title/description shape is configurable without a code
  change — matches "same title and description structure" across apps.

### New package: internal/deploycreate

- `googledocs.go` (or a small `internal/googledocs/` package): fetch a
  Document by ID via the Docs API, walk `Body.Content` for the first `Table`,
  match header cells to "Application"/"PR URL(s)" (case-insensitive, order-
  agnostic), and return `[]Row{AppName string; PRRefs []string}` — splitting
  each PR-URL cell on newlines/commas, trimming, and validating each looks
  like a `github.com/.../pull/\d+` URL.
- `session.go` / new storage table `deploy_sessions` (id PK, chat_id,
  message_id, status, rows as a JSON blob, created_at) — needed because the
  parsed rows (many apps, many URLs) don't fit in Telegram's 64-byte
  `callback_data`, and must survive a pod restart between "show summary" and
  "user taps Confirm", same reasoning as `cicd_retries` in phase 1's data
  model. Repository methods: `CreateDeploySession`, `GetDeploySession`,
  `UpdateDeploySessionStatus`, `ExpirePendingDeploySessions`.
- `handlers.go`:
  - `HandleCommand` (triggered by `/deploy <url>`): extract the Doc ID from
    the URL, fetch+parse the table, resolve each `AppName` against
    `cfg.Repos` (by `DisplayName`/`Name`), persist a `deploy_sessions` row,
    and reply with a summary (matched apps + their PR-ref counts, and any
    unmatched rows called out) plus Confirm/Cancel buttons
    (`telegrambot.EncodeCallback("deploy", "confirm", sessionID)` — small
    payload, well under the 64-byte limit since it's just an ID).
  - `HandleCallback`: on Confirm, loop the persisted rows, call the existing
    `githubapp.Client.CreatePR` (`internal/githubapp/client.go` — no changes
    needed) once per matched row using the configured source/target branch
    and the title/body templates, updating each row's status; edit the
    message with a final per-row result list (success link or error). On
    Cancel, mark the session cancelled and strip the keyboard
    (`telegrambot.Client.RemoveKeyboard`).
- `sweep.go`: reuse the same 24h/30min periodic-expiry shape as phase 1's
  `internal/retry/sweep.go` for abandoned `deploy_sessions` awaiting
  confirmation.

### Wiring

- `internal/httpserver/telegram_webhook.go`: add a `/deploy` prefix check in
  `routeMessage` and a `"deploy"` case in `routeCallback`'s feature switch.
- `cmd/bot/main.go`: construct the new `deploycreate.Handler` (store, tg,
  gh, cfg.Repos, branch config, the Google Docs client, logger) and start
  its sweep goroutine, the same way `retryH` is wired today.

### Verification (once implemented)

- `go build ./...` / `go vet ./...` stay clean; add unit tests for the Docs
  table parser (feed it a fixed `docs.Document` JSON fixture with header
  variations, multiple PR URLs per cell, and an unmatched app name) the same
  way `internal/notify/format_test.go` tests pure functions today.
- Manually share a real test Google Doc (with a table) with the service
  account, run `/deploy <link>` against a dev bot + test repos, confirm the
  summary message lists the right matched/unmatched rows, tap Confirm, and
  verify one PR gets created per matched row with the configured
  title/description shape.
