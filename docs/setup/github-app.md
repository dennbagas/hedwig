# GitHub App Setup

Hedwig needs one GitHub App, installed on whichever repositories you want notifications for. This is a one-time, operational (not code) setup — do this before filling in `github.*` config.

## 1. Create the app

1. Go to [github.com/settings/apps/new](https://github.com/settings/apps/new) (for a personal account) or your organization's **Settings → Developer settings → GitHub Apps → New GitHub App**.
2. **GitHub App name**: anything (e.g. `hedwig-yourorg`).
3. **Homepage URL**: anything reachable — it's not used by Hedwig, GitHub just requires a value.
4. **Webhook**:
   - **Active**: checked.
   - **Webhook URL**: `https://<your-domain>/webhooks/github` — this path is fixed (not configurable), unlike Telegram's/Slack's webhook paths.
   - **Secret**: generate a random value (e.g. `openssl rand -hex 32`) and keep it — this is `github.webhook_secret` / `APP_GITHUB_WEBHOOK_SECRET`. Hedwig uses it to verify the HMAC-SHA256 signature GitHub attaches to every webhook delivery (`githubapp.ValidateWebhook`).
5. **Permissions** — under **Repository permissions**, grant read access for whichever event types you want notifications for (see the table below); everything else can stay "No access." **Actions** must be **Read and write**, not just read, if you want the CI/CD retry button (`retry.enabled: true`) — Hedwig's retry button calls GitHub's rerun-failed-jobs endpoint, which needs write access, not just the ability to receive `workflow_run` events.
6. **Subscribe to events** — check the boxes matching the event types below.
7. **Where can this GitHub App be installed?** — "Only on this account" is fine unless you need it across multiple orgs.
8. Click **Create GitHub App**.

## 2. Required permissions and events

Only enable what you actually need — Hedwig silently ignores webhook deliveries for event types it has no handler registered for, but GitHub won't send an event at all if you didn't subscribe to it or grant the underlying permission.

| Notification | Webhook event to subscribe to | Repository permission needed |
|---|---|---|
| Push | `Pushes` | Contents: Read-only |
| PR opened/merged | `Pull requests` | Pull requests: Read-only |
| PR comment | `Issue comments` | Issues: Read-only (PR conversation comments are delivered as `issue_comment` events) |
| PR review | `Pull request reviews` | Pull requests: Read-only |
| PR review comment | `Pull request review comments` | Pull requests: Read-only |
| Branch created | `Branch or tag creation` | Contents: Read-only |
| Release published | `Releases` | Contents: Read-only |
| CI/CD status + retry button | `Workflow runs` | Actions: **Read and write** (write is required for the retry button's rerun-failed-jobs call; read-only breaks `retry.enabled: true`) |

If you're not using the retry button (`retry.enabled: false`, the default), Actions: Read-only is enough for `workflow_run` notifications.

## 3. Generate the private key

On the app's settings page, under **Private keys**, click **Generate a private key**. This downloads a `.pem` file — save it somewhere Hedwig's config can reference it via `github.private_key_path` (a file path, not the key contents inline). In Kubernetes this file is mounted from a Secret (see the [Kubernetes deployment guide](../deployments/kubernetes.md)).

## 4. Note the App ID

Still on the app's settings page, the **App ID** is shown near the top — this is `github.app_id` in config.

## 5. Install the app and get the Installation ID

1. On the app's settings page, go to **Install App** (left sidebar) and install it on the account/org, choosing either "All repositories" or specific ones.
2. After installing, the URL you land on looks like `https://github.com/settings/installations/<INSTALLATION_ID>` (or `.../organizations/<org>/settings/installations/<INSTALLATION_ID>` for an org) — that numeric ID is `github.installation_id`.
   - Alternatively, call `GET /app/installations` (authenticated as the app via a JWT) and read `id` from the response.

## Result

You should now have four values for the `github:` section of `config.yaml`:

```yaml
github:
  app_id: <App ID, step 4>
  installation_id: <Installation ID, step 5>
  private_key_path: /path/to/the/downloaded.pem   # step 3
  webhook_secret: ""   # set via APP_GITHUB_WEBHOOK_SECRET, step 1
```
