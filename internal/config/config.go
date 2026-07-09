package config

type Config struct {
	Server   ServerConfig   `koanf:"server"`
	GitHub   GitHubConfig   `koanf:"github"`
	Telegram TelegramConfig `koanf:"telegram"`
	Repos    []RepoConfig   `koanf:"repos"    validate:"required,min=1,dive,required"`
	PR       PRConfig       `koanf:"pr"`
	Database DatabaseConfig `koanf:"database"`
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

type RepoConfig struct {
	Name  string `koanf:"name"  validate:"required"`
	Owner string `koanf:"owner" validate:"required"`
}

type PRConfig struct {
	SourceBranch string `koanf:"source_branch" validate:"required"`
	TargetBranch string `koanf:"target_branch" validate:"required"`
}

type DatabaseConfig struct {
	Path string `koanf:"path" validate:"required"`
}
