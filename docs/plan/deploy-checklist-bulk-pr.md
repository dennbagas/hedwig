# Plan: Bulk PR creation from a Google Doc deployment checklist

Status: proposed (design only, not yet implemented)

## Context

Today, `/newpr` (`internal/prcreate/`) creates exactly one PR per Telegram
conversation, with the user typing the title and description by hand at each
step. In practice, deployments involve a fixed group of applications whose
release PRs all follow the same title/description shape — a reference list of
the individual PR URLs going into that release. Typing that out per app in the
existing wizard is repetitive.

The team already tracks each deployment in a human-edited Google Doc
("deployment checklist") listing, per application, the PR URL(s) being
shipped. The goal: forward that doc's link to the bot and have it open one
release PR per application automatically, using those PR URLs as the
description content — without turning the checklist into something that
looks/feels like a data file to the humans who maintain it.

Decisions already made with the user:
- **Format**: keep the checklist as a Google Doc, but require a **table**
  inside it (App | PR URL(s) column headers) — the Docs API exposes tables as
  structured rows/cells, so this parses as reliably as a spreadsheet while the
  doc still reads naturally, with prose allowed around the table.
- **Meaning of the PR URLs**: they are reference links to embed in the new
  release PR's description (changelog/traceability) — hedwig does not need to
  inspect or verify the state of those linked PRs.
- **Interface**: Telegram, via a new command (e.g. `/deploy <doc-url>`),
  reusing the existing bot identity and the confirm-before-create pattern
  already used by `/newpr`.

Defaults assumed below (flag if different behavior is wanted):
- **Partial failures**: continue best-effort through all rows and report a
  per-row result at the end, rather than aborting the whole batch on one
  failure — a broken row shouldn't block the rest of the deploy.
- **Unmatched app names**: rows whose "Application" text doesn't match any
  configured repo are skipped and called out in the confirmation summary
  (not silently dropped, not a hard error) — user can bail out via Cancel if
  something looks wrong before anything is created.

## New dependency & credentials

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

## Config additions

- `internal/config/config.go`: new `GoogleConfig{ServiceAccountKeyPath
  string `validate:"required,file"`}` field on `Config`, validated the same
  way `GitHubConfig.PrivateKeyPath` is in `internal/config/load.go`.
- `RepoConfig` gets an optional `DisplayName` field used to match the
  checklist doc's "Application" column text case-insensitively; falls back to
  `Name` when unset. This keeps one repo list shared by both `/newpr` and
  `/deploy` instead of a second list.
- Add a `pr.deploy_title_template` / `pr.deploy_body_template` pair (Go
  `text/template` strings, e.g. `{{.App}}` / `{{.PRRefs}}`) to `PRConfig` so
  the release PR's title/description shape is configurable without a code
  change — matches "same title and description structure" from the request.

## New package: internal/deploycreate (mirrors internal/prcreate's shape)

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
  "user taps Confirm", same reasoning as `pr_sessions`/`cicd_retries` in
  `internal/storage/sqlite_repository.go`. Add matching `Repository`
  methods: `CreateDeploySession`, `GetDeploySession`,
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
    (`telegrambot.Client.RemoveKeyboard`), same as
    `prcreate/handlers.go:handleCancelled`.
- `sweep.go`: reuse the same 24h/30min periodic-expiry shape as
  `internal/retry/sweep.go` / `internal/prcreate/sweep.go` for abandoned
  `deploy_sessions` awaiting confirmation.

## Wiring

- `internal/httpserver/telegram_webhook.go`: extend `routeMessage` with a
  `/deploy` prefix check (alongside the existing `/newpr` check) and extend
  `routeCallback`'s feature switch with a `"deploy"` case, mirroring the
  existing `"pr"` case.
- `cmd/bot/main.go`: construct the new `deploycreate.Handler` (store, tg,
  gh, cfg.Repos, branch config, the Google Docs client, logger) and start
  its sweep goroutine, the same way `retryH`/`prH` are wired today.

## Verification (once implemented)

- `go build ./...` / `go vet ./...` stay clean; add unit tests for the Docs
  table parser (feed it a fixed `docs.Document` JSON fixture with header
  variations, multiple PR URLs per cell, and an unmatched app name) the same
  way `internal/notify/format_test.go` tests pure functions today.
- Manually share a real test Google Doc (with a table) with the service
  account, run `/deploy <link>` against a dev bot + test repos, confirm the
  summary message lists the right matched/unmatched rows, tap Confirm, and
  verify one PR gets created per matched row with the configured
  title/description shape.
