# Kubernetes Deployment

This guide covers deploying Hedwig on Kubernetes using the container image from GitHub Container Registry.

## Prerequisites

- Kubernetes cluster with `kubectl` access
- A namespace to deploy into (examples use `hedwig`)
- The GitHub App private key (`.pem` file)
- Bot token and webhook secrets from Telegram and/or Slack, and from GitHub
- At least one of Telegram or Slack must be enabled — see [docs/setup/telegram.md](../setup/telegram.md) / [docs/setup/slack.md](../setup/slack.md) for how to get the values each needs, and [docs/setup/github-app.md](../setup/github-app.md) for the GitHub App

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

    # Both channels are individually optional (either can be disabled/omitted);
    # the only requirement is that at least one of telegram.enabled /
    # slack.enabled is true.
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

    # Opt-in: set retry.enabled: true to add a "Retry failed jobs" button to
    # workflow_run failure notifications. Defaults to false (plain messages,
    # no GitHub rerun call). Requires the Slack Interactivity Request URL
    # below if Slack is enabled — the button doesn't work without it.
    retry:
      enabled: false
```

### Notification templates

Telegram and Slack templates live in **separate ConfigMaps** — `hedwig-templates-telegram`
and `hedwig-templates-slack` — so either can be edited/rotated independently (e.g. tweak
Slack's wording without touching Telegram's, or vice versa). Ready-to-apply examples, kept
in sync with the repo's actual default templates, are checked in at
[`docs/deployments/configmap-templates-telegram.yaml`](configmap-templates-telegram.yaml)
and [`docs/deployments/configmap-templates-slack.yaml`](configmap-templates-slack.yaml):

```bash
kubectl apply -n hedwig -f docs/deployments/configmap-templates-telegram.yaml
kubectl apply -n hedwig -f docs/deployments/configmap-templates-slack.yaml
```

Or generate them directly from the repo's [`templates/`](../templates/) directory (every
`<event>.tmpl` file is Telegram; every `<event>.slack.tmpl` file is Slack). `kubectl create
configmap --from-file` only accepts a whole directory at once, which would mix both
platforms together, so pass individual files instead to split them:

```bash
telegram_args=()
for f in ./templates/*.tmpl; do
  [[ "$f" == *.slack.tmpl ]] && continue
  telegram_args+=(--from-file="$f")
done
kubectl create configmap hedwig-templates-telegram --namespace hedwig "${telegram_args[@]}"

slack_args=()
for f in ./templates/*.slack.tmpl; do
  slack_args+=(--from-file="$f")
done
kubectl create configmap hedwig-templates-slack --namespace hedwig "${slack_args[@]}"
```

If that's too fiddly, just apply the checked-in YAML files instead.

Both ConfigMaps are mounted into the **same** `/etc/hedwig/templates` directory via a
[projected volume](#deployment) below — `templateLoader` scans one flat directory
regardless of which ConfigMap a file came from, keyed by `<event>` for Telegram and
`<event>.slack` for Slack.

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
          # optional: true on both sources so this doesn't block pod
          # startup if you only created one platform's ConfigMap (e.g.
          # Telegram-only, no hedwig-templates-slack at all).
          projected:
            sources:
              - configMap:
                  name: hedwig-templates-telegram
                  optional: true
              - configMap:
                  name: hedwig-templates-slack
                  optional: true
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

Creating the Slack app itself (bot token, signing secret, Interactivity Request URL, channel ID) is covered in [docs/setup/slack.md](../setup/slack.md) — it's the same steps regardless of how you deploy Hedwig. Once you have those values, set `slack.enabled: true` in the `hedwig-config` ConfigMap and add `slack-bot-token`/`slack-signing-secret` to `hedwig-secrets` as shown above, then roll the deployment (`kubectl rollout restart deployment/hedwig -n hedwig`) to pick up the change.

## Updating notification templates

Templates are mounted from ConfigMaps, so they can be changed without restarting the pod.
Edit the relevant checked-in YAML file and re-apply it — e.g. for a Slack wording change:

```bash
kubectl apply -n hedwig -f docs/deployments/configmap-templates-slack.yaml

# The pod picks up changes automatically (kubelet syncs ConfigMap mounts ~1 min)
```

(Same for `configmap-templates-telegram.yaml` on the Telegram side.) Editing only the
platform you changed avoids restating both ConfigMaps' full content for a one-line
wording tweak.
