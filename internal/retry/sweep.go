package retry

import (
	"context"
	"time"

	"hedwig/internal/database"
	"hedwig/internal/telegrambot"

	"github.com/rs/zerolog"
)

// RunSweep runs a periodic goroutine that expires pending retry buttons older than expiry.
// It returns when ctx is cancelled.
func RunSweep(ctx context.Context, store database.Repository, tg telegrambot.Client, interval, expiry time.Duration, logger zerolog.Logger) {
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

func sweep(ctx context.Context, store database.Repository, tg telegrambot.Client, expiry time.Duration, logger zerolog.Logger) {
	retries, err := store.ExpirePendingRetries(ctx, expiry)
	if err != nil {
		logger.Error().Err(err).Msg("expire pending retries")
		return
	}
	for _, r := range retries {
		if err := tg.RemoveKeyboard(ctx, r.ChatID, r.MessageID); err != nil {
			logger.Warn().Err(err).Int64("retry_id", r.ID).Msg("remove expired retry keyboard")
		}
	}
}
