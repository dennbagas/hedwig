package prcreate

import (
	"context"
	"time"

	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
	"go.uber.org/zap"
)

// RunSweep periodically expires in-progress PR sessions older than expiry.
func RunSweep(ctx context.Context, store storage.Repository, tg telegrambot.Client, interval, expiry time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepSessions(ctx, store, tg, expiry, logger)
		}
	}
}

func sweepSessions(ctx context.Context, store storage.Repository, tg telegrambot.Client, expiry time.Duration, logger *zap.Logger) {
	sessions, err := store.ExpireInProgressSessions(ctx, expiry)
	if err != nil {
		logger.Error("expire in-progress pr sessions", zap.Error(err))
		return
	}
	for _, s := range sessions {
		if err := tg.RemoveKeyboard(ctx, s.ChatID, s.MessageID); err != nil {
			logger.Warn("remove expired pr session keyboard", zap.Error(err))
		}
	}
}
