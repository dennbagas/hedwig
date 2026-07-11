package retry

import (
	"context"
	"fmt"
	"strconv"

	"github.com/btse/hedwig/internal/githubapp"
	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
	"go.uber.org/zap"
)

const (
	callbackFeature = "retry"
	callbackAction  = "trigger"
)

// Handler manages CI/CD retry state and Telegram interactions.
type Handler struct {
	store  storage.Repository
	tg     telegrambot.Client
	github githubapp.Client
	logger *zap.Logger
}

func New(store storage.Repository, tg telegrambot.Client, gh githubapp.Client, logger *zap.Logger) *Handler {
	return &Handler{store: store, tg: tg, github: gh, logger: logger}
}

// NotifyFailure sends the CI/CD failure message with a retry button and persists the retry record.
func (h *Handler) NotifyFailure(ctx context.Context, chatID int64, workflowName, owner, repo string, runID int64, runURL string) error {
	text := fmt.Sprintf("CI/CD failed: <b>%s</b>\n%s/%s\n<a href=\"%s\">View run</a>",
		workflowName, owner, repo, runURL)

	// Send placeholder first to get the messageID, then we update with button.
	msgID, err := h.tg.SendMessage(ctx, chatID, text, telegrambot.WithParseMode("HTML"))
	if err != nil {
		return fmt.Errorf("send failure notification: %w", err)
	}

	// Persist the retry record before attaching the button so we have the ID for callback data.
	retryID, err := h.store.CreateRetry(ctx, storage.CICDRetry{
		ChatID:    chatID,
		MessageID: msgID,
		RunID:     runID,
		Repo:      fmt.Sprintf("%s/%s", owner, repo),
		Status:    storage.RetryStatusPending,
	})
	if err != nil {
		return fmt.Errorf("store retry record: %w", err)
	}

	callbackData := telegrambot.EncodeCallback(callbackFeature, callbackAction, strconv.FormatInt(retryID, 10))
	btn := [][]telegrambot.Button{{
		{Text: "Retry failed jobs", CallbackData: callbackData},
	}}

	if err := h.tg.EditMessage(ctx, chatID, msgID, text,
		telegrambot.WithParseMode("HTML"),
		telegrambot.WithInlineKeyboard(btn)); err != nil {
		h.logger.Warn("failed to attach retry button", zap.Error(err), zap.Int64("retry_id", retryID))
	}
	return nil
}

// HandleCallback processes a "Retry failed jobs" button tap.
func (h *Handler) HandleCallback(ctx context.Context, callbackQueryID string, chatID, messageID, retryID int64) error {
	// Answer immediately to remove the loading spinner.
	_ = h.tg.AnswerCallback(ctx, callbackQueryID, "")

	rec, err := h.store.GetRetry(ctx, retryID)
	if err != nil {
		return fmt.Errorf("get retry record: %w", err)
	}
	if rec == nil || rec.Status != storage.RetryStatusPending {
		return h.tg.EditMessage(ctx, chatID, messageID, "This retry button is no longer valid.")
	}

	// Parse owner/repo from stored "owner/repo" string.
	owner, repo := splitRepo(rec.Repo)

	if err := h.github.RerunFailedJobs(ctx, owner, repo, rec.RunID); err != nil {
		h.logger.Error("rerun failed jobs API error", zap.Error(err), zap.Int64("run_id", rec.RunID))
		text := fmt.Sprintf("Failed to retry: %v\n<a href=\"https://github.com/%s/actions/runs/%d\">Check on GitHub</a>",
			err, rec.Repo, rec.RunID)
		return h.tg.EditMessage(ctx, chatID, messageID, text, telegrambot.WithParseMode("HTML"))
	}

	if err := h.store.UpdateRetryStatus(ctx, retryID, storage.RetryStatusRetried); err != nil {
		h.logger.Warn("failed to update retry status to retried", zap.Error(err))
	}

	h.logger.Info("retry triggered", zap.Int64("retry_id", retryID), zap.Int64("run_id", rec.RunID), zap.String("repo", rec.Repo))
	return h.tg.EditMessage(ctx, chatID, messageID, "Retrying failed jobs…")
}

func splitRepo(ownerRepo string) (owner, repo string) {
	for i, c := range ownerRepo {
		if c == '/' {
			return ownerRepo[:i], ownerRepo[i+1:]
		}
	}
	return ownerRepo, ""
}
