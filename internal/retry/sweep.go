package retry

import (
	"context"
	"time"

	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
	"go.uber.org/zap"
)

// RunSweep runs a periodic goroutine that expires pending retry buttons older than expiry.
// It returns when ctx is cancelled.
func RunSweep(ctx context.Context, store storage.Repository, tg telegrambot.Client, interval, expiry time.Duration, logger *zap.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep(ctx, store, tg, expiry, logger)
		}
	}
}

func sweep(ctx context.Context, store storage.Repository, tg telegrambot.Client, expiry time.Duration, logger *zap.Logger) {
	retries, err := store.ExpirePendingRetries(ctx, expiry)
	if err != nil {
		logger.Error("expire pending retries", zap.Error(err))
		return
	}
	for _, r := range retries {
		if err := tg.RemoveKeyboard(ctx, r.ChatID, r.MessageID); err != nil {
			logger.Warn("remove expired retry keyboard", zap.Error(err), zap.Int64("retry_id", r.ID))
		}
	}
}
