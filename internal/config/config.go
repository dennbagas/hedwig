package config

type Config struct {
	Server        ServerConfig        `koanf:"server"`
	GitHub        GitHubConfig        `koanf:"github"`
	Telegram      TelegramConfig      `koanf:"telegram"`
	Database      DatabaseConfig      `koanf:"database"`
	Logging       LoggingConfig       `koanf:"logging"`
	Notifications NotificationsConfig `koanf:"notifications"`
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

type TelegramConfig struct {
	BotToken       string  `koanf:"bot_token"       validate:"required"`
	WebhookSecret  string  `koanf:"webhook_secret"  validate:"required"`
	WebhookPath    string  `koanf:"webhook_path"    validate:"required"`
	WebhookURL     string  `koanf:"webhook_url"     validate:"required,url"`
	ChatID         int64   `koanf:"chat_id"         validate:"required"`
	AllowedUserIDs []int64 `koanf:"allowed_user_ids" validate:"required,min=1"`
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

