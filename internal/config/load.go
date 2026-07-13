package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/btse/hedwig/internal/telegrambot"
	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

func Load(path string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("load config file: %w", err)
	}

	if err := k.Load(env.Provider("APP_", ".", func(s string) string {
		return strings.ToLower(strings.TrimPrefix(s, "APP_"))
	}), nil); err != nil {
		return nil, fmt.Errorf("load env vars: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validator.New().Struct(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	if err := validateRepoCallbackDataLen(cfg.Repos); err != nil {
		return nil, err
	}

	if _, err := loadPrivateKey(cfg.GitHub.PrivateKeyPath); err != nil {
		return nil, fmt.Errorf("parse github private key: %w", err)
	}

	return &cfg, nil
}

// validateRepoCallbackDataLen fails fast if any configured repo's "owner/name"
// would push the prcreate repo-selection callback data over Telegram's
// hard 64-byte callback_data limit, which would otherwise only surface at
// runtime as a rejected sendMessage/editMessageText call when /newpr is used.
func validateRepoCallbackDataLen(repos []RepoConfig) error {
	for _, r := range repos {
		data := telegrambot.EncodeCallback("pr", "repo", r.Owner+"/"+r.Name)
		if len(data) > telegrambot.MaxCallbackDataLen {
			return fmt.Errorf("repo %s/%s: callback data too long (%d bytes, max %d)",
				r.Owner, r.Name, len(data), telegrambot.MaxCallbackDataLen)
		}
	}
	return nil
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseRSAPrivateKey(pemBytes)
}

func ParseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// try PKCS8
		parsed, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse PKCS1: %v; parse PKCS8: %v", err, err2)
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("expected RSA private key, got %T", parsed)
		}
		return rsaKey, nil
	}
	return key, nil
}
