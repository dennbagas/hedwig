# Kubernetes Deployment

This guide covers deploying Hedwig on Kubernetes using the container image from GitHub Container Registry.

## Prerequisites

- Kubernetes cluster with `kubectl` access
- A namespace to deploy into (examples use `hedwig`)
- The GitHub App private key (`.pem` file)
- Bot token and webhook secrets from Telegram and GitHub

## Secrets

Hedwig requires four secrets. Create them as a single Kubernetes Secret:

```bash
kubectl create namespace hedwig

kubectl create secret generic hedwig-secrets \
  --namespace hedwig \
  --from-literal=telegram-bot-token='<your-telegram-bot-token>' \
  --from-literal=telegram-webhook-secret='<your-telegram-webhook-secret>' \
  --from-literal=github-webhook-secret='<your-github-webhook-secret>' \
  --from-file=github-app.pem=./secrets/github-app.pem
```

| Key | Description |
|---|---|
| `telegram-bot-token` | Bot token from @BotFather |
| `telegram-webhook-secret` | Random secret you set when registering the Telegram webhook URL |
| `github-webhook-secret` | HMAC secret configured in your GitHub App webhook settings |
| `github-app.pem` | GitHub App private key file |

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
      bot_token: ""        # set via APP_TELEGRAM_BOT_TOKEN
      webhook_secret: ""   # set via APP_TELEGRAM_WEBHOOK_SECRET
      webhook_path: /webhooks/telegram
      webhook_url: https://hedwig.example.com/webhooks/telegram
      chat_id: -100123456789
      allowed_user_ids:
        - 111222333

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
  pull_request.tmpl: |
    {{- if eq .Action "opened" -}}
    📢 New Pull Request!

    Title: {{.Title}}
    Author: {{.Author}}

    Pull Request URL:
    {{.URL}}
    {{- end -}}
  # add remaining event templates here — see templates/ in the repo for defaults
```

Copy the defaults from the repo's [`templates/`](../templates/) directory as a starting point:

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

Replace `hedwig.example.com` with your actual domain. The domain must be reachable by both GitHub (for webhook delivery) and Telegram (for webhook registration).

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
