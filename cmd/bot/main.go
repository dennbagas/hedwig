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
	"hedwig/internal/telegrambot"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	defaultConfig := "config.yaml"
	if v := os.Getenv("APP_CONFIG"); v != "" {
		defaultConfig = v
	}
	configPath := flag.String("config", defaultConfig, "path to config file")
	flag.Parse()

	logger := logging.New("info")

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal().Err(err).Msg("load config")
	}

	logger = logging.New(cfg.Logging.Level)

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	store := database.NewSQLiteRepository(db)

	ghHTTPClient, err := githubapp.NewInstallationHTTPClient(
		cfg.GitHub.AppID, cfg.GitHub.InstallationID, cfg.GitHub.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("create github http client: %w", err)
	}

	gh, err := githubapp.New(ghHTTPClient, cfg.GitHub.WebhookSecret)
	if err != nil {
		return fmt.Errorf("create github client: %w", err)
	}

	tg, err := telegrambot.New(cfg.Telegram.BotToken)
	if err != nil {
		return fmt.Errorf("create telegram bot: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := tg.SetWebhook(ctx, cfg.Telegram.WebhookURL, cfg.Telegram.WebhookSecret); err != nil {
		return fmt.Errorf("set telegram webhook: %w", err)
	}

	retryH := retry.New(store, tg, gh, logger)

	notifyD := notify.NewDispatcher(tg, cfg.Telegram.ChatID, logger)
	notify.RegisterAll(notifyD, tg, retryH, cfg.Telegram.ChatID)

	srv := httpserver.New(
		gh, store, notifyD, retryH,
		cfg.Telegram.AllowedUserIDs,
		cfg.Telegram.WebhookSecret,
		cfg.Server.HealthzPath,
		cfg.Telegram.WebhookPath,
		logger,
	)

	go retry.RunSweep(ctx, store, tg, 30*time.Minute, 24*time.Hour, logger)

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
