package retry

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"hedwig/internal/database"
	"hedwig/internal/githubapp"
	"hedwig/internal/slackbot"
	"hedwig/internal/telegrambot"

	"github.com/rs/zerolog"
)

const (
	// CallbackFeature is the callback feature name for the CI/CD retry
	// button, used to identify it in both the Telegram and Slack webhook
	// routers. It is not gated by allowedUserIDs / any per-platform
	// authorization — anyone who can see the button (i.e. anyone in the
	// notified chat/channel) may tap it.
	CallbackFeature = "retry"
	callbackAction  = "trigger"
)

// FailureText holds the pre-rendered failure notification text for each
// platform — kept separate because Telegram HTML and Slack mrkdwn render
// the same event as different strings, not a single shared format.
type FailureText struct {
	Telegram string // empty if Telegram is disabled or its template skipped this event
	Slack    string // empty if Slack is disabled or its template skipped this event
}

// Handler manages CI/CD retry state and fans out to whichever of
// Telegram/Slack are configured. Either client may be nil (that platform
// disabled); every send is skipped gracefully when its client is nil.
type Handler struct {
	store       database.Repository
	tg          telegrambot.Client
	slack       slackbot.Client
	slackChanID string
	github      githubapp.Client
	logger      zerolog.Logger
}

func New(store database.Repository, tg telegrambot.Client, slack slackbot.Client, slackChanID string, gh githubapp.Client, logger zerolog.Logger) *Handler {
	return &Handler{store: store, tg: tg, slack: slack, slackChanID: slackChanID, github: gh, logger: logger}
}

// NotifyFailure sends the failure message (with a retry button) to every
// enabled platform and persists one CICDRetry row plus one RetryTarget row
// per platform actually posted to.
func (h *Handler) NotifyFailure(ctx context.Context, chatID int64, workflowName, owner, repo string, runID int64, text FailureText) error {
	retryID, err := h.store.CreateRetry(ctx, database.CICDRetry{
		RunID:        runID,
		Repo:         fmt.Sprintf("%s/%s", owner, repo),
		WorkflowName: workflowName,
		Status:       database.RetryStatusPending,
	})
	if err != nil {
		return fmt.Errorf("store retry record: %w", err)
	}

	callbackData := telegrambot.EncodeCallback(CallbackFeature, callbackAction, strconv.FormatInt(retryID, 10))

	if h.tg != nil && text.Telegram != "" {
		if err := h.notifyTelegram(ctx, chatID, retryID, callbackData, text.Telegram); err != nil {
			h.logger.Error().Err(err).Int64("retry_id", retryID).Msg("failed to post telegram failure notification")
		}
	}
	if h.slack != nil && text.Slack != "" {
		if err := h.notifySlack(ctx, retryID, callbackData, text.Slack); err != nil {
			h.logger.Error().Err(err).Int64("retry_id", retryID).Msg("failed to post slack failure notification")
		}
	}

	return nil
}

func (h *Handler) notifyTelegram(ctx context.Context, chatID, retryID int64, callbackData, text string) error {
	msgID, err := h.tg.SendMessage(ctx, chatID, text, telegrambot.WithParseMode("HTML"))
	if err != nil {
		return fmt.Errorf("send telegram failure notification: %w", err)
	}

	if err := h.store.CreateRetryTarget(ctx, database.RetryTarget{
		RetryID: retryID, Platform: database.PlatformTelegram,
		ChatRef: strconv.FormatInt(chatID, 10), MessageRef: strconv.FormatInt(msgID, 10),
		MessageText: text,
	}); err != nil {
		return fmt.Errorf("store telegram retry target: %w", err)
	}

	btn := [][]telegrambot.Button{{{Text: "Retry failed jobs", CallbackData: callbackData}}}
	if err := h.tg.EditMessage(ctx, chatID, msgID, text,
		telegrambot.WithParseMode("HTML"),
		telegrambot.WithInlineKeyboard(btn)); err != nil {
		h.logger.Warn().Err(err).Int64("retry_id", retryID).Msg("failed to attach telegram retry button")
	}
	return nil
}

func (h *Handler) notifySlack(ctx context.Context, retryID int64, callbackData, text string) error {
	buttons := []slackbot.Button{{Text: "Retry failed jobs", Value: callbackData}}
	ts, err := h.slack.PostMessage(ctx, h.slackChanID, text, buttons)
	if err != nil {
		return fmt.Errorf("send slack failure notification: %w", err)
	}

	if err := h.store.CreateRetryTarget(ctx, database.RetryTarget{
		RetryID: retryID, Platform: database.PlatformSlack,
		ChatRef: h.slackChanID, MessageRef: ts,
		MessageText: text,
	}); err != nil {
		return fmt.Errorf("store slack retry target: %w", err)
	}
	return nil
}

// HandleCallback processes a "Retry failed jobs" button tap from either
// platform. callbackQueryID is Telegram's callback query ID to answer (empty
// when the tap came from Slack). platform/chatRef/messageRef identify the
// specific message that was tapped — needed even when retryID turns out to
// be unknown to the store (e.g. a stale/garbage button), since there is then
// nothing to look up in cicd_retry_targets and the only message we can
// correct is the one the tap came from.
func (h *Handler) HandleCallback(ctx context.Context, callbackQueryID, platform, chatRef, messageRef string, retryID int64) error {
	if callbackQueryID != "" && h.tg != nil {
		_ = h.tg.AnswerCallback(ctx, callbackQueryID, "")
	}

	rec, err := h.store.GetRetry(ctx, retryID)
	if err != nil {
		return fmt.Errorf("get retry record: %w", err)
	}
	if rec == nil {
		// Nothing was ever stored for this ID — fan out to a synthetic
		// single target built from the tapped message itself, since
		// cicd_retry_targets has no rows to look up.
		h.fanOut(ctx, []database.RetryTarget{{RetryID: retryID, Platform: platform, ChatRef: chatRef, MessageRef: messageRef}},
			func(database.RetryTarget) string { return "This retry button is no longer valid." }, true)
		return nil
	}

	targets, err := h.store.ListRetryTargets(ctx, retryID)
	if err != nil {
		return fmt.Errorf("list retry targets: %w", err)
	}
	if rec.Status != database.RetryStatusPending {
		h.fanOut(ctx, targets, func(database.RetryTarget) string {
			return "This retry button is no longer valid."
		}, true)
		return nil
	}

	owner, repo := splitRepo(rec.Repo)

	if err := h.github.RerunFailedJobs(ctx, owner, repo, rec.RunID); err != nil {
		h.logger.Error().Err(err).Int64("run_id", rec.RunID).Msg("rerun failed jobs API error")
		checkURL := fmt.Sprintf("https://github.com/%s/actions/runs/%d", rec.Repo, rec.RunID)
		h.fanOut(ctx, targets, func(t database.RetryTarget) string {
			if t.Platform == database.PlatformSlack {
				return fmt.Sprintf("Failed to retry: %s\n<%s|Check on GitHub>", err.Error(), checkURL)
			}
			return fmt.Sprintf("Failed to retry: %s\n<a href=\"%s\">Check on GitHub</a>",
				html.EscapeString(err.Error()), html.EscapeString(checkURL))
		}, false)
		return nil
	}

	if err := h.store.UpdateRetryStatus(ctx, retryID, database.RetryStatusRetried); err != nil {
		h.logger.Warn().Err(err).Msg("failed to update retry status to retried")
	}

	h.logger.Info().Int64("retry_id", retryID).Int64("run_id", rec.RunID).Str("repo", rec.Repo).Msg("retry triggered")
	h.fanOut(ctx, targets, func(t database.RetryTarget) string {
		return strings.TrimRight(t.MessageText, "\n") + "\n\n✅ Retry request sent"
	}, true)
	return nil
}

// fanOut edits every target's message to buildText(target), stripping its
// button when clearKeyboard is true. When clearKeyboard is false the button
// must keep working (e.g. a rerun API error — the user should be able to
// tap retry again), so it is re-attached with the same callback value
// rather than left alone: Telegram preserves an omitted keyboard option
// automatically, but Slack's chat.update clears blocks whenever text is
// provided and blocks are omitted (verified against the Slack API docs), so
// both platforms are handled the same explicit way here rather than relying
// on that Telegram-only omission behavior. Errors are logged, not returned —
// a failure updating one platform's message must not stop the others.
func (h *Handler) fanOut(ctx context.Context, targets []database.RetryTarget, buildText func(database.RetryTarget) string, clearKeyboard bool) {
	for _, t := range targets {
		text := buildText(t)
		callbackData := telegrambot.EncodeCallback(CallbackFeature, callbackAction, strconv.FormatInt(t.RetryID, 10))

		switch t.Platform {
		case database.PlatformTelegram:
			if h.tg == nil {
				continue
			}
			chatID, err1 := strconv.ParseInt(t.ChatRef, 10, 64)
			msgID, err2 := strconv.ParseInt(t.MessageRef, 10, 64)
			if err1 != nil || err2 != nil {
				h.logger.Error().Int64("retry_id", t.RetryID).Msg("malformed telegram retry target refs")
				continue
			}
			keyboard := [][]telegrambot.Button{{{Text: "Retry failed jobs", CallbackData: callbackData}}}
			if clearKeyboard {
				keyboard = [][]telegrambot.Button{}
			}
			if err := h.tg.EditMessage(ctx, chatID, msgID, text,
				telegrambot.WithParseMode("HTML"),
				telegrambot.WithInlineKeyboard(keyboard)); err != nil {
				h.logger.Warn().Err(err).Int64("retry_id", t.RetryID).Msg("failed to update telegram retry message")
			}
		case database.PlatformSlack:
			if h.slack == nil {
				continue
			}
			var buttons []slackbot.Button
			if !clearKeyboard {
				buttons = []slackbot.Button{{Text: "Retry failed jobs", Value: callbackData}}
			}
			if err := h.slack.UpdateMessage(ctx, t.ChatRef, t.MessageRef, text, buttons); err != nil {
				h.logger.Warn().Err(err).Int64("retry_id", t.RetryID).Msg("failed to update slack retry message")
			}
		default:
			h.logger.Warn().Str("platform", t.Platform).Msg("unknown retry target platform")
		}
	}
}

func splitRepo(ownerRepo string) (owner, repo string) {
	for i, c := range ownerRepo {
		if c == '/' {
			return ownerRepo[:i], ownerRepo[i+1:]
		}
	}
	return ownerRepo, ""
}
