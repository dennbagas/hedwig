package retry

import (
	"context"
	"strings"
	"time"

	"hedwig/internal/database"
)

// RunSweep runs a periodic goroutine that expires pending retry buttons
// older than expiry, across every platform each retry was posted to. It
// returns when ctx is cancelled.
func RunSweep(ctx context.Context, h *Handler, interval, expiry time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep(ctx, h, expiry)
		}
	}
}

func sweep(ctx context.Context, h *Handler, expiry time.Duration) {
	retries, err := h.store.ExpirePendingRetries(ctx, expiry)
	if err != nil {
		h.logger.Error().Err(err).Msg("expire pending retries")
		return
	}
	for _, r := range retries {
		targets, err := h.store.ListRetryTargets(ctx, r.ID)
		if err != nil {
			h.logger.Error().Err(err).Int64("retry_id", r.ID).Msg("list retry targets for expiry sweep")
			continue
		}
		h.fanOut(ctx, targets, func(t database.RetryTarget) string {
			return strings.TrimRight(t.MessageText, "\n") + "\n\n⌛ This retry button has expired."
		}, true)
	}
}
