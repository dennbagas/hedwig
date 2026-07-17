package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// validate is a package-level singleton per the library's own guidance:
// "Validate is designed to be thread-safe and used as a singleton instance
// ... Using multiple instances neglects the benefit of caching."
var validate = validator.New()

func Load(path string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(confmap.Provider(map[string]any{
		"logging.level":    "info",
		"server.port":      8080,
		"server.healthz_path": "/healthz",
	}, "."), nil); err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("load config file: %w", err)
	}

	if err := k.Load(env.Provider("APP_", ".", func(s string) string {
		s = strings.ToLower(strings.TrimPrefix(s, "APP_"))
		// Only the first underscore separates the top-level config section
		// (server, github, telegram, pr, database) from the field path below
		// it; field names themselves are snake_case and must keep their
		// remaining underscores (e.g. "webhook_secret", "allowed_user_ids").
		return strings.Replace(s, "_", ".", 1)
	}), nil); err != nil {
		return nil, fmt.Errorf("load env vars: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validate.Struct(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	if _, err := loadPrivateKey(cfg.GitHub.PrivateKeyPath); err != nil {
		return nil, fmt.Errorf("parse github private key: %w", err)
	}

	return &cfg, nil
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
