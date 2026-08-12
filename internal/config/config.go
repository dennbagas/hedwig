package config

type Config struct {
	Server        ServerConfig        `koanf:"server"`
	GitHub        GitHubConfig        `koanf:"github"`
	Telegram      TelegramConfig      `koanf:"telegram"`
	Slack         SlackConfig         `koanf:"slack"`
	Database      DatabaseConfig      `koanf:"database"`
	Logging       LoggingConfig       `koanf:"logging"`
	Notifications NotificationsConfig `koanf:"notifications"`
	Retry         RetryConfig         `koanf:"retry"`
}

type ServerConfig struct {
	Port        int    `koanf:"port"         validate:"required,min=1,max=65535"`
	HealthzPath string `koanf:"healthz_path" validate:"required"`
}

type GitHubConfig struct {
	AppID          int64  `koanf:"app_id"           validate:"required"`
	InstallationID int64  `koanf:"installation_id"  validate:"required"`
	PrivateKeyPath string `koanf:"private_key_path" validate:"required,file"`
	WebhookSecret  string `koanf:"webhook_secret"   validate:"required"`
}

// TelegramConfig fields are only required when Enabled is true; see
// validateChannels in load.go for the conditional enforcement (kept manual
// rather than `required_if` struct tags because WebhookURL also needs
// format validation, and omitempty/required_if ordering with url is
// fragile to get right in struct tags).
type TelegramConfig struct {
	Enabled       bool   `koanf:"enabled"`
	BotToken      string `koanf:"bot_token"`
	WebhookSecret string `koanf:"webhook_secret"`
	WebhookPath   string `koanf:"webhook_path"`
	WebhookURL    string `koanf:"webhook_url"`
	ChatID        int64  `koanf:"chat_id"`
}

// SlackConfig fields are only required when Enabled is true; see
// validateChannels in load.go.
type SlackConfig struct {
	Enabled       bool   `koanf:"enabled"`
	BotToken      string `koanf:"bot_token"`
	SigningSecret string `koanf:"signing_secret"`
	ChannelID     string `koanf:"channel_id"`
	WebhookPath   string `koanf:"webhook_path"`
}

type DatabaseConfig struct {
	Path string `koanf:"path" validate:"required"`
}

type LoggingConfig struct {
	Level string `koanf:"level"`
}

type NotificationsConfig struct {
	TemplatesDir string `koanf:"templates_dir" validate:"required,dir"`
}

// RetryConfig gates the CI/CD "Retry failed jobs" button. Defaults to
// disabled (Go zero value) — workflow_run failure notifications are plain
// messages (no button, no GitHub rerun call) unless explicitly opted in.
type RetryConfig struct {
	Enabled bool `koanf:"enabled"`
}
