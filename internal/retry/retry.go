package retry

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"hedwig/internal/database"
	"hedwig/internal/githubapp"
	"hedwig/internal/telegrambot"

	"github.com/rs/zerolog"
)

const (
	callbackFeature = "retry"
	callbackAction  = "trigger"
)

// Handler manages CI/CD retry state and Telegram interactions.
type Handler struct {
	store  database.Repository
	tg     telegrambot.Client
	github githubapp.Client
	logger zerolog.Logger
}

func New(store database.Repository, tg telegrambot.Client, gh githubapp.Client, logger zerolog.Logger) *Handler {
	return &Handler{store: store, tg: tg, github: gh, logger: logger}
}

// NotifyFailure sends a pre-rendered failure message with a retry button and
// persists the retry record. text is the HTML rendered by the notification
// template; it is stored so the button-click handler can append to it later.
func (h *Handler) NotifyFailure(ctx context.Context, chatID int64, workflowName, owner, repo string, runID int64, text string) error {
	msgID, err := h.tg.SendMessage(ctx, chatID, text, telegrambot.WithParseMode("HTML"))
	if err != nil {
		return fmt.Errorf("send failure notification: %w", err)
	}

	retryID, err := h.store.CreateRetry(ctx, database.CICDRetry{
		ChatID:       chatID,
		MessageID:    msgID,
		RunID:        runID,
		Repo:         fmt.Sprintf("%s/%s", owner, repo),
		WorkflowName: workflowName,
		MessageText:  text,
		Status:       database.RetryStatusPending,
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
		h.logger.Warn().Err(err).Int64("retry_id", retryID).Msg("failed to attach retry button")
	}
	return nil
}

// HandleCallback processes a "Retry failed jobs" button tap.
func (h *Handler) HandleCallback(ctx context.Context, callbackQueryID string, chatID, messageID, retryID int64) error {
	_ = h.tg.AnswerCallback(ctx, callbackQueryID, "")

	rec, err := h.store.GetRetry(ctx, retryID)
	if err != nil {
		return fmt.Errorf("get retry record: %w", err)
	}
	if rec == nil || rec.Status != database.RetryStatusPending {
		return h.tg.EditMessage(ctx, chatID, messageID, "This retry button is no longer valid.")
	}

	owner, repo := splitRepo(rec.Repo)

	if err := h.github.RerunFailedJobs(ctx, owner, repo, rec.RunID); err != nil {
		h.logger.Error().Err(err).Int64("run_id", rec.RunID).Msg("rerun failed jobs API error")
		text := fmt.Sprintf("Failed to retry: %s\n<a href=\"https://github.com/%s/actions/runs/%d\">Check on GitHub</a>",
			html.EscapeString(err.Error()), html.EscapeString(rec.Repo), rec.RunID)
		return h.tg.EditMessage(ctx, chatID, messageID, text, telegrambot.WithParseMode("HTML"))
	}

	if err := h.store.UpdateRetryStatus(ctx, retryID, database.RetryStatusRetried); err != nil {
		h.logger.Warn().Err(err).Msg("failed to update retry status to retried")
	}

	h.logger.Info().Int64("retry_id", retryID).Int64("run_id", rec.RunID).Str("repo", rec.Repo).Msg("retry triggered")
	text := strings.TrimRight(rec.MessageText, "\n") + "\n\n✅ Retry request sent"
	return h.tg.EditMessage(ctx, chatID, messageID, text, telegrambot.WithParseMode("HTML"))
}

func splitRepo(ownerRepo string) (owner, repo string) {
	for i, c := range ownerRepo {
		if c == '/' {
			return ownerRepo[:i], ownerRepo[i+1:]
		}
	}
	return ownerRepo, ""
}
