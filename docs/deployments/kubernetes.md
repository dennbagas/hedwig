# Kubernetes Deployment

This guide covers deploying Hedwig on Kubernetes using the container image from GitHub Container Registry.

## Prerequisites

- Kubernetes cluster with `kubectl` access
- A namespace to deploy into (examples use `hedwig`)
- The GitHub App private key (`.pem` file)
- Bot token and webhook secrets from Telegram and/or Slack, and from GitHub
- At least one of Telegram or Slack must be enabled — see [Slack app setup](#slack-app-setup) if you're enabling Slack

## Secrets

Create the secrets Hedwig needs as a single Kubernetes Secret. Include only the
Telegram and/or Slack keys for the channel(s) you're enabling — `github-webhook-secret`
and `github-app.pem` are always required:

```bash
kubectl create namespace hedwig

kubectl create secret generic hedwig-secrets \
  --namespace hedwig \
  --from-literal=telegram-bot-token='<your-telegram-bot-token>' \
  --from-literal=telegram-webhook-secret='<your-telegram-webhook-secret>' \
  --from-literal=slack-bot-token='<your-slack-bot-token>' \
  --from-literal=slack-signing-secret='<your-slack-signing-secret>' \
  --from-literal=github-webhook-secret='<your-github-webhook-secret>' \
  --from-file=github-app.pem=./secrets/github-app.pem
```

| Key | Description | Required when |
|---|---|---|
| `telegram-bot-token` | Bot token from @BotFather | `telegram.enabled: true` |
| `telegram-webhook-secret` | Random secret you set when registering the Telegram webhook URL | `telegram.enabled: true` |
| `slack-bot-token` | Bot User OAuth Token (`xoxb-...`) from your Slack app's **OAuth & Permissions** page | `slack.enabled: true` |
| `slack-signing-secret` | Signing Secret from your Slack app's **Basic Information** page | `slack.enabled: true` |
| `github-webhook-secret` | HMAC secret configured in your GitHub App webhook settings | always |
| `github-app.pem` | GitHub App private key file | always |

## ConfigMap

The app config and notification templates are mounted as ConfigMaps so they can be updated without rebuilding the image.

### config.yaml

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hedwig-config
  namespace: hedwig
data:
  config.yaml: |
    server:
      port: 8080
      healthz_path: /healthz

    github:
      app_id: 123456
      installation_id: 789012
      private_key_path: /etc/hedwig/secrets/github-app.pem
      webhook_secret: ""   # set via APP_GITHUB_WEBHOOK_SECRET

    telegram:
      enabled: true
      bot_token: ""        # set via APP_TELEGRAM_BOT_TOKEN
      webhook_secret: ""   # set via APP_TELEGRAM_WEBHOOK_SECRET
      webhook_path: /webhooks/telegram
      webhook_url: https://hedwig.example.com/webhooks/telegram
      chat_id: -100123456789

    # Slack is optional — omit or set slack.enabled: false to notify
    # Telegram only. At least one of telegram.enabled / slack.enabled
    # must be true.
    slack:
      enabled: true
      bot_token: ""          # set via APP_SLACK_BOT_TOKEN
      signing_secret: ""     # set via APP_SLACK_SIGNING_SECRET
      channel_id: "C0123456789"
      webhook_path: /webhooks/slack/interactions

    database:
      path: /data/hedwig.db

    logging:
      level: info

    notifications:
      templates_dir: /etc/hedwig/templates
```

### Notification templates

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hedwig-templates
  namespace: hedwig
data:
  push.tmpl: |
    ✅ Push to <code>{{.Ref}}</code>
    Author: {{.Pusher}}
    Message: {{.Summary}}
    Repository: {{.Repo}}
  push.slack.tmpl: |
    ✅ Push to `{{.Ref}}`
    Author: {{.Pusher}}
    Message: {{.Summary}}
    Repository: {{.Repo}}
  pull_request.tmpl: |
    {{- if eq .Action "opened" -}}
    📢 New Pull Request!

    Title: {{.Title}}
    Author: {{.Author}}

    Pull Request URL:
    {{.URL}}
    {{- end -}}
  # add remaining event templates here — see templates/ in the repo for defaults.
  # Every event type needs two files if Slack is enabled: <event>.tmpl
  # (Telegram, HTML) and <event>.slack.tmpl (Slack, mrkdwn) — Hedwig renders
  # each independently and skips whichever platform's template is missing
  # or produces empty output.
```

Copy the defaults from the repo's [`templates/`](../templates/) directory as a starting point — this picks up every `*.tmpl` file, including the `*.slack.tmpl` ones, in one command:

```bash
kubectl create configmap hedwig-templates \
  --namespace hedwig \
  --from-file=./templates/
```

## Persistent Volume

The SQLite database needs a persistent volume. A simple PVC:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: hedwig-data
  namespace: hedwig
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

## Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hedwig
  namespace: hedwig
spec:
  replicas: 1
  selector:
    matchLabels:
      app: hedwig
  template:
    metadata:
      labels:
        app: hedwig
    spec:
      containers:
        - name: hedwig
          image: ghcr.io/dennbagas/hedwig:v1.0.0
          args: ["-config", "/etc/hedwig/config.yaml"]
          ports:
            - containerPort: 8080
          env:
            - name: APP_GITHUB_WEBHOOK_SECRET
              valueFrom:
                secretKeyRef:
                  name: hedwig-secrets
                  key: github-webhook-secret
            - name: APP_TELEGRAM_BOT_TOKEN
              valueFrom:
                secretKeyRef:
                  name: hedwig-secrets
                  key: telegram-bot-token
            - name: APP_TELEGRAM_WEBHOOK_SECRET
              valueFrom:
                secretKeyRef:
                  name: hedwig-secrets
                  key: telegram-webhook-secret
            # optional: true lets the pod start even if you didn't create
            # these keys (e.g. Slack disabled) — omit these two env vars
            # entirely if you're never going to enable Slack.
            - name: APP_SLACK_BOT_TOKEN
              valueFrom:
                secretKeyRef:
                  name: hedwig-secrets
                  key: slack-bot-token
                  optional: true
            - name: APP_SLACK_SIGNING_SECRET
              valueFrom:
                secretKeyRef:
                  name: hedwig-secrets
                  key: slack-signing-secret
                  optional: true
          volumeMounts:
            - name: config
              mountPath: /etc/hedwig/config.yaml
              subPath: config.yaml
            - name: templates
              mountPath: /etc/hedwig/templates
            - name: secrets
              mountPath: /etc/hedwig/secrets
              readOnly: true
            - name: data
              mountPath: /data
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 10
      volumes:
        - name: config
          configMap:
            name: hedwig-config
        - name: templates
          configMap:
            name: hedwig-templates
        - name: secrets
          secret:
            secretName: hedwig-secrets
            items:
              - key: github-app.pem
                path: github-app.pem
        - name: data
          persistentVolumeClaim:
            claimName: hedwig-data
```

> **Replicas:** Keep this at `1`. SQLite does not support concurrent writers, so multiple replicas will corrupt the database.

## Service and Ingress

```yaml
apiVersion: v1
kind: Service
metadata:
  name: hedwig
  namespace: hedwig
spec:
  selector:
    app: hedwig
  ports:
    - port: 80
      targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: hedwig
  namespace: hedwig
spec:
  rules:
    - host: hedwig.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: hedwig
                port:
                  number: 80
```

Replace `hedwig.example.com` with your actual domain. The domain must be reachable by GitHub (for webhook delivery), Telegram (for webhook registration), and — if Slack is enabled — Slack (for the Interactivity Request URL, `/webhooks/slack/interactions`).

## Slack app setup

Slack notifications and the interactive retry button require a Slack app configured before you fill in `slack.*` config/secrets. This is a one-time, operational (not code) setup:

1. Create a Slack app at [api.slack.com/apps](https://api.slack.com/apps) (from scratch or a manifest) in the target workspace.
2. Under **OAuth & Permissions**, add the `chat:write` Bot Token Scope (add `chat:write.public` too if you don't want to invite the bot to the channel first). Install the app to the workspace and copy the **Bot User OAuth Token** (`xoxb-...`) — this is `slack-bot-token` / `APP_SLACK_BOT_TOKEN`.
3. Under **Basic Information**, copy the **Signing Secret** — this is `slack-signing-secret` / `APP_SLACK_SIGNING_SECRET`. Hedwig uses it to verify that interaction requests actually came from Slack (HMAC signature check on `/webhooks/slack/interactions`).
4. Under **Interactivity & Shortcuts**, turn Interactivity on and set the **Request URL** to `https://hedwig.example.com/webhooks/slack/interactions` (your Ingress host + the `slack.webhook_path` from config). Slack sends the CI/CD retry button's tap events here.
5. Invite the bot to the channel you want notifications in (`/invite @your-bot-name`), then get that channel's ID (right-click the channel → **View channel details**, ID is at the bottom) — this is `slack.channel_id` in config.

Once these are in place, set `slack.enabled: true` in the `hedwig-config` ConfigMap and add the two secret keys to `hedwig-secrets` as shown above, then roll the deployment (`kubectl rollout restart deployment/hedwig -n hedwig`) to pick up the change.

## Updating notification templates

Templates are mounted from a ConfigMap, so they can be changed without restarting the pod:

```bash
# Edit a template locally, then replace the ConfigMap
kubectl create configmap hedwig-templates \
  --namespace hedwig \
  --from-file=./templates/ \
  --dry-run=client -o yaml | kubectl apply -f -

# The pod picks up changes automatically (kubelet syncs ConfigMap mounts ~1 min)
```
