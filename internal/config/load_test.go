package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func rsaPKCS1PEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
}

func rsaPKCS8PEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func ed25519PKCS8PEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// baseYAML returns a fully valid config, with every field overridable so
// individual tests can break exactly one thing.
func baseYAML(o overrides) string {
	o.applyDefaults()
	return fmt.Sprintf(`
server:
  port: %s
  healthz_path: /healthz

github:
  app_id: 123456
  installation_id: 789012
  private_key_path: %s
  webhook_secret: %q

telegram:
  bot_token: "test-bot-token"
  webhook_secret: "test-telegram-secret"
  webhook_path: /webhooks/telegram
  webhook_url: https://hedwig.example.com/webhooks/telegram
  chat_id: -100123456789

database:
  path: /data/hedwig.db

notifications:
  templates_dir: %s
`, o.port, o.keyPath, o.githubWebhookSecret, o.templatesDir)
}

type overrides struct {
	port                string
	keyPath             string
	githubWebhookSecret string
	templatesDir        string
}

func (o *overrides) applyDefaults() {
	if o.port == "" {
		o.port = "8080"
	}
	if o.githubWebhookSecret == "" {
		o.githubWebhookSecret = "test-github-secret"
	}
	if o.templatesDir == "" {
		o.templatesDir = os.TempDir()
	}
}

func TestLoadSuccess(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))
	// Leave github.webhook_secret empty in the file, matching the documented
	// "secrets go via env var, not the file" pattern, and confirm the env
	// var actually fills it in.
	cfgPath := writeFile(t, dir, "config.yaml", []byte(baseYAML(overrides{
		keyPath:             keyPath,
		githubWebhookSecret: "",
	})))

	t.Setenv("APP_GITHUB_WEBHOOK_SECRET", "from-env-secret")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.GitHub.WebhookSecret != "from-env-secret" {
		t.Errorf("GitHub.WebhookSecret = %q, want env override %q", cfg.GitHub.WebhookSecret, "from-env-secret")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.GitHub.AppID != 123456 {
		t.Errorf("GitHub.AppID = %d, want 123456", cfg.GitHub.AppID)
	}
}

func TestLoadEnvOverridesEveryDocumentedField(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))
	cfgPath := writeFile(t, dir, "config.yaml", []byte(baseYAML(overrides{keyPath: keyPath})))

	t.Setenv("APP_SERVER_PORT", "9090")
	t.Setenv("APP_GITHUB_WEBHOOK_SECRET", "env-github-secret")
	t.Setenv("APP_TELEGRAM_BOT_TOKEN", "env-bot-token")
	t.Setenv("APP_TELEGRAM_WEBHOOK_SECRET", "env-telegram-secret")
	t.Setenv("APP_DATABASE_PATH", "/env/hedwig.db")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090 (env override)", cfg.Server.Port)
	}
	if cfg.GitHub.WebhookSecret != "env-github-secret" {
		t.Errorf("GitHub.WebhookSecret = %q, want env override", cfg.GitHub.WebhookSecret)
	}
	if cfg.Telegram.BotToken != "env-bot-token" {
		t.Errorf("Telegram.BotToken = %q, want env override", cfg.Telegram.BotToken)
	}
	if cfg.Telegram.WebhookSecret != "env-telegram-secret" {
		t.Errorf("Telegram.WebhookSecret = %q, want env override", cfg.Telegram.WebhookSecret)
	}
	if cfg.Database.Path != "/env/hedwig.db" {
		t.Errorf("Database.Path = %q, want env override", cfg.Database.Path)
	}
}

func TestLoadValidationFailures(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))

	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "port out of range",
			yaml: baseYAML(overrides{keyPath: keyPath, port: "70000"}),
		},
		{
			name: "port zero (also fails required)",
			yaml: baseYAML(overrides{keyPath: keyPath, port: "0"}),
		},
		{
			name: "nonexistent private key path",
			yaml: baseYAML(overrides{keyPath: filepath.Join(dir, "does-not-exist.pem")}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgPath := writeFile(t, t.TempDir(), "config.yaml", []byte(tt.yaml))
			_, err := Load(cfgPath)
			if err == nil {
				t.Fatal("Load() error = nil, want a validation error")
			}
			if !strings.Contains(err.Error(), "validate config") {
				t.Errorf("Load() error = %q, want it to come from validation", err.Error())
			}
		})
	}
}

func TestLoadMissingRequiredField(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))
	yaml := fmt.Sprintf(`
server:
  port: 8080
  healthz_path: /healthz

github:
  installation_id: 789012
  private_key_path: %s
  webhook_secret: "s"

telegram:
  bot_token: "t"
  webhook_secret: "s"
  webhook_path: /webhooks/telegram
  webhook_url: https://hedwig.example.com/webhooks/telegram
  chat_id: -100123456789

database:
  path: /data/hedwig.db
`, keyPath) // github.app_id omitted entirely -> zero value -> fails "required"
	cfgPath := writeFile(t, dir, "config.yaml", []byte(yaml))

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing github.app_id")
	}
	if !strings.Contains(err.Error(), "validate config") {
		t.Errorf("Load() error = %q, want it to come from validation", err.Error())
	}
}

func TestLoadNoChannelEnabled(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))
	yaml := fmt.Sprintf(`
server:
  port: 8080
  healthz_path: /healthz

github:
  app_id: 123456
  installation_id: 789012
  private_key_path: %s
  webhook_secret: "s"

telegram:
  enabled: false

slack:
  enabled: false

database:
  path: /data/hedwig.db

notifications:
  templates_dir: %s
`, keyPath, os.TempDir())
	cfgPath := writeFile(t, dir, "config.yaml", []byte(yaml))

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() error = nil, want error when no channel is enabled")
	}
	if !strings.Contains(err.Error(), "validate config") {
		t.Errorf("Load() error = %q, want it to come from validation", err.Error())
	}
}

func TestLoadSlackEnabledMissingFields(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))
	yaml := fmt.Sprintf(`
server:
  port: 8080
  healthz_path: /healthz

github:
  app_id: 123456
  installation_id: 789012
  private_key_path: %s
  webhook_secret: "s"

telegram:
  enabled: false

slack:
  enabled: true

database:
  path: /data/hedwig.db

notifications:
  templates_dir: %s
`, keyPath, os.TempDir())
	cfgPath := writeFile(t, dir, "config.yaml", []byte(yaml))

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() error = nil, want error for slack.enabled with no bot_token/signing_secret/channel_id/webhook_path")
	}
	if !strings.Contains(err.Error(), "validate config") {
		t.Errorf("Load() error = %q, want it to come from validation", err.Error())
	}
}

func TestLoadSlackOnlySuccess(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))
	yaml := fmt.Sprintf(`
server:
  port: 8080
  healthz_path: /healthz

github:
  app_id: 123456
  installation_id: 789012
  private_key_path: %s
  webhook_secret: "s"

telegram:
  enabled: false

slack:
  enabled: true
  bot_token: "xoxb-test"
  signing_secret: "test-signing-secret"
  channel_id: "C0123456789"
  webhook_path: /webhooks/slack/interactions

database:
  path: /data/hedwig.db

notifications:
  templates_dir: %s
`, keyPath, os.TempDir())
	cfgPath := writeFile(t, dir, "config.yaml", []byte(yaml))

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v, want success with Telegram disabled and Slack fully configured", err)
	}
	if cfg.Telegram.Enabled {
		t.Error("Telegram.Enabled = true, want false")
	}
	if !cfg.Slack.Enabled {
		t.Error("Slack.Enabled = false, want true")
	}
	if cfg.Slack.ChannelID != "C0123456789" {
		t.Errorf("Slack.ChannelID = %q, want C0123456789", cfg.Slack.ChannelID)
	}
}

func TestLoadTelegramEnabledDefaultsToTrue(t *testing.T) {
	// baseYAML never sets telegram.enabled explicitly; it must default to
	// true so upgrading deployments keep notifying Telegram without any
	// config change.
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))
	cfgPath := writeFile(t, dir, "config.yaml", []byte(baseYAML(overrides{keyPath: keyPath})))

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Telegram.Enabled {
		t.Error("Telegram.Enabled = false, want true (default)")
	}
	if cfg.Slack.Enabled {
		t.Error("Slack.Enabled = true, want false (default)")
	}
}

func TestLoadRetryEnabledDefaultsToFalse(t *testing.T) {
	// baseYAML never sets retry.enabled explicitly; it must default to false
	// — the retry button is opt-in, not on by default.
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))
	cfgPath := writeFile(t, dir, "config.yaml", []byte(baseYAML(overrides{keyPath: keyPath})))

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Retry.Enabled {
		t.Error("Retry.Enabled = true, want false (default)")
	}
}

func TestLoadRetryEnabledExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	keyPath := writeFile(t, dir, "key.pem", rsaPKCS1PEM(t))
	yaml := baseYAML(overrides{keyPath: keyPath}) + "\nretry:\n  enabled: true\n"
	cfgPath := writeFile(t, dir, "config.yaml", []byte(yaml))

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Retry.Enabled {
		t.Error("Retry.Enabled = false, want true (explicit override)")
	}
}

func TestLoadMalformedPrivateKey(t *testing.T) {
	dir := t.TempDir()
	badKeyPath := writeFile(t, dir, "bad-key.pem", []byte("not a pem file at all"))
	cfgPath := writeFile(t, dir, "config.yaml", []byte(baseYAML(overrides{keyPath: badKeyPath})))

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() error = nil, want error for malformed PEM")
	}
	if !strings.Contains(err.Error(), "parse github private key") {
		t.Errorf("Load() error = %q, want it to come from private key parsing", err.Error())
	}
}

func TestLoadNonexistentConfigFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing config file")
	}
}

func TestParseRSAPrivateKey(t *testing.T) {
	t.Run("valid PKCS1", func(t *testing.T) {
		key, err := ParseRSAPrivateKey(rsaPKCS1PEM(t))
		if err != nil {
			t.Fatalf("ParseRSAPrivateKey() error = %v", err)
		}
		if key == nil {
			t.Fatal("ParseRSAPrivateKey() returned nil key")
		}
	})

	t.Run("valid PKCS8", func(t *testing.T) {
		key, err := ParseRSAPrivateKey(rsaPKCS8PEM(t))
		if err != nil {
			t.Fatalf("ParseRSAPrivateKey() error = %v", err)
		}
		if key == nil {
			t.Fatal("ParseRSAPrivateKey() returned nil key")
		}
	})

	t.Run("no PEM block", func(t *testing.T) {
		_, err := ParseRSAPrivateKey([]byte("this is not PEM data"))
		if err == nil {
			t.Fatal("ParseRSAPrivateKey() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "no PEM block found") {
			t.Errorf("ParseRSAPrivateKey() error = %q, want mention of missing PEM block", err.Error())
		}
	})

	t.Run("PKCS8 key that isn't RSA", func(t *testing.T) {
		_, err := ParseRSAPrivateKey(ed25519PKCS8PEM(t))
		if err == nil {
			t.Fatal("ParseRSAPrivateKey() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "expected RSA private key") {
			t.Errorf("ParseRSAPrivateKey() error = %q, want mention of expecting an RSA key", err.Error())
		}
	})
}
