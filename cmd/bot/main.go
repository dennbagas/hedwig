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

	"github.com/btse/hedwig/internal/config"
	"github.com/btse/hedwig/internal/githubapp"
	"github.com/btse/hedwig/internal/httpserver"
	"github.com/btse/hedwig/internal/logging"
	"github.com/btse/hedwig/internal/notify"
	"github.com/btse/hedwig/internal/prcreate"
	"github.com/btse/hedwig/internal/retry"
	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	logger, err := logging.New()
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer logger.Sync() //nolint:errcheck

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	db, err := storage.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	store := storage.NewSQLiteRepository(db)

	ghHTTPClient, err := githubapp.NewInstallationHTTPClient(
		cfg.GitHub.AppID, cfg.GitHub.InstallationID, cfg.GitHub.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("create github http client: %w", err)
	}

	gh := githubapp.New(ghHTTPClient, cfg.GitHub.WebhookSecret)

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
	prH := prcreate.New(store, tg, gh, cfg.Repos, cfg.PR.SourceBranch, cfg.PR.TargetBranch, logger)

	notifyD := notify.NewDispatcher(tg, cfg.Telegram.ChatID, logger)
	notify.RegisterAll(notifyD, tg, retryH, cfg.Telegram.ChatID)

	srv := httpserver.New(
		gh, store, notifyD, retryH, prH,
		cfg.Telegram.AllowedUserIDs,
		cfg.Telegram.WebhookSecret,
		cfg.Server.HealthzPath,
		cfg.Telegram.WebhookPath,
		logger,
	)

	go retry.RunSweep(ctx, store, tg, 30*time.Minute, 24*time.Hour, logger)
	go prcreate.RunSweep(ctx, store, tg, 30*time.Minute, 24*time.Hour, logger)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("starting server", zap.String("addr", addr))

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
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
