package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hedwig/internal/config"
	"hedwig/internal/database"
	"hedwig/internal/githubapp"
	"hedwig/internal/httpserver"
	"hedwig/internal/logging"
	"hedwig/internal/notify"
	"hedwig/internal/retry"
	"hedwig/internal/slackbot"
	"hedwig/internal/telegrambot"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Config path defaults to config.yaml; override with APP_CONFIG env var or -config flag.
	defaultConfig := "config.yaml"
	if v := os.Getenv("APP_CONFIG"); v != "" {
		defaultConfig = v
	}
	configPath := flag.String("config", defaultConfig, "path to config file")
	flag.Parse()

	// Bootstrap with info-level logger until we can read the configured level.
	logger := logging.New("info")

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal().Err(err).Msg("load config")
	}

	logger = logging.New(cfg.Logging.Level)

	// SQLite database — holds webhook delivery dedup records and pending CI retry state.
	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	store := database.NewSQLiteRepository(db)

	// GitHub App client authenticated as an installation, used to validate webhooks and trigger reruns.
	ghHTTPClient, err := githubapp.NewInstallationHTTPClient(
		cfg.GitHub.AppID, cfg.GitHub.InstallationID, cfg.GitHub.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("create github http client: %w", err)
	}

	gh, err := githubapp.New(ghHTTPClient, cfg.GitHub.WebhookSecret)
	if err != nil {
		return fmt.Errorf("create github client: %w", err)
	}

	// tg and slack are nil when their channel is disabled — every downstream
	// consumer (notify handlers, retry.Handler) treats a nil client as
	// "skip this platform".
	var tg telegrambot.Client
	if cfg.Telegram.Enabled {
		tg, err = telegrambot.New(cfg.Telegram.BotToken)
		if err != nil {
			return fmt.Errorf("create telegram bot: %w", err)
		}
	}

	var slack slackbot.Client
	if cfg.Slack.Enabled {
		slack = slackbot.New(cfg.Slack.BotToken)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Register the webhook URL with Telegram so it knows where to send updates.
	if cfg.Telegram.Enabled {
		if err := tg.SetWebhook(ctx, cfg.Telegram.WebhookURL, cfg.Telegram.WebhookSecret); err != nil {
			return fmt.Errorf("set telegram webhook: %w", err)
		}
	}

	// retry.Handler manages CI/CD failure notifications with inline retry buttons.
	retryH := retry.New(store, tg, slack, cfg.Slack.ChannelID, gh, logger)

	// notify.Dispatcher routes each GitHub event type to its handler using templates loaded from disk.
	notifyD, err := notify.New(tg, cfg.Telegram.ChatID, slack, cfg.Slack.ChannelID, retryH, cfg.Notifications.TemplatesDir, logger)
	if err != nil {
		return fmt.Errorf("load notification templates: %w", err)
	}

	srv := httpserver.New(
		gh, store, notifyD, retryH,
		cfg.Telegram.WebhookSecret,
		cfg.Server.HealthzPath,
		cfg.Telegram.WebhookPath,
		cfg.Slack.Enabled,
		cfg.Slack.SigningSecret,
		cfg.Slack.WebhookPath,
		logger,
	)

	// Background goroutine expires retry buttons older than 24 h every 30 minutes.
	go retry.RunSweep(ctx, retryH, 30*time.Minute, 24*time.Hour)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info().Msg(fmt.Sprintf("starting http server on port: %d", cfg.Server.Port))

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown: wait for SIGINT/SIGTERM then give in-flight requests 10 s to finish.
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}
