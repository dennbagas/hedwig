package retry

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"hedwig/internal/database"
	"hedwig/internal/githubapp"
	"hedwig/internal/slackbot"
	"hedwig/internal/telegrambot"

	"github.com/google/go-github/v88/github"
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
	// enabled gates HandleCallback, not just NotifyFailure: a retry record
	// created before retry was disabled (or one that outlives a toggle) must
	// not be able to trigger RerunFailedJobs once disabled.
	enabled bool
	logger  zerolog.Logger
}

func New(store database.Repository, tg telegrambot.Client, slack slackbot.Client, slackChanID string, gh githubapp.Client, enabled bool, logger zerolog.Logger) *Handler {
	return &Handler{store: store, tg: tg, slack: slack, slackChanID: slackChanID, github: gh, enabled: enabled, logger: logger}
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

	if !h.enabled {
		// Retry is disabled: reject the tap outright rather than looking up
		// the record, so a pending row created before retry was disabled (or
		// left over across a toggle) can never reach RerunFailedJobs.
		h.fanOut(ctx, []database.RetryTarget{{RetryID: retryID, Platform: platform, ChatRef: chatRef, MessageRef: messageRef}},
			func(database.RetryTarget) string { return "This retry button is no longer valid." }, true)
		return nil
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

	// Atomically claim the retry before calling GitHub: the same retry can
	// have a live button on both Telegram and Slack simultaneously, so two
	// near-simultaneous taps could otherwise both observe "pending" and
	// trigger RerunFailedJobs twice. Only the caller that wins the claim
	// proceeds; a lost race is treated the same as "already handled."
	claimed, err := h.store.ClaimPendingRetry(ctx, retryID, database.RetryStatusRetried)
	if err != nil {
		return fmt.Errorf("claim pending retry: %w", err)
	}
	if !claimed {
		h.fanOut(ctx, targets, func(database.RetryTarget) string {
			return "This retry button is no longer valid."
		}, true)
		return nil
	}

	owner, repo := splitRepo(rec.Repo)

	if err := h.github.RerunFailedJobs(ctx, owner, repo, rec.RunID); err != nil {
		h.logger.Error().Err(err).Int64("run_id", rec.RunID).Msg("rerun failed jobs API error")
		// Non-retryable rejections (e.g. "This workflow run cannot be
		// retried") mean tapping the button again will just fail the same
		// way, so leave status as "retried" and drop the button entirely.
		// Other confirmed rejections (e.g. 404 Not Found, 422 Unprocessable
		// Entity for a genuinely transient/fixable cause) reset to pending
		// so the button stays usable. Ambiguous errors (network timeouts,
		// 5xx, non-ErrorResponse errors) also leave status as "retried" to
		// prevent double-trigger races — we can't be sure whether GitHub
		// actually accepted the rerun despite the error. The button is only
		// re-attached when the status was actually reset to pending —
		// otherwise a second tap would just hit the "no longer valid" branch,
		// so keeping the button visible in that case would be misleading.
		nonRetryable := isNonRetryableRun(err)
		resetToPending := isConfirmedRejection(err) && !nonRetryable
		if resetToPending {
			if resetErr := h.store.UpdateRetryStatus(ctx, retryID, database.RetryStatusPending); resetErr != nil {
				h.logger.Warn().Err(resetErr).Int64("retry_id", retryID).Msg("failed to reset retry status to pending after confirmed rejection")
				// The record's actual status is still "retried" since the
				// update didn't take — the button must not be re-attached
				// for a state that was never actually reached.
				resetToPending = false
			}
		}
		checkURL := fmt.Sprintf("https://github.com/%s/actions/runs/%d", rec.Repo, rec.RunID)
		h.fanOut(ctx, targets, func(t database.RetryTarget) string {
			if nonRetryable {
				if t.Platform == database.PlatformSlack {
					return fmt.Sprintf("This workflow run cannot be retried. Please open the GitHub Action directly.\n<%s|Open GitHub Action>", checkURL)
				}
				return fmt.Sprintf("This workflow run cannot be retried. Please open the GitHub Action directly.\n<a href=\"%s\">Open GitHub Action</a>",
					html.EscapeString(checkURL))
			}
			if t.Platform == database.PlatformSlack {
				return fmt.Sprintf("Failed to retry: %s\n<%s|Check on GitHub>", escapeSlackMrkdwn(err.Error()), checkURL)
			}
			return fmt.Sprintf("Failed to retry: %s\n<a href=\"%s\">Check on GitHub</a>",
				html.EscapeString(err.Error()), html.EscapeString(checkURL))
		}, !resetToPending)
		return nil
	}

	h.logger.Info().Int64("retry_id", retryID).Int64("run_id", rec.RunID).Str("repo", rec.Repo).Msg("retry triggered")
	h.fanOut(ctx, targets, func(t database.RetryTarget) string {
		return strings.TrimRight(t.MessageText, "\n") + "\n\n✅ Retry request sent"
	}, true)
	return nil
}

// isConfirmedRejection returns true when err is a structured GitHub API
// error response with a status code indicating the request was definitively
// rejected (404 Not Found, 422 Unprocessable Entity, etc.). Returns false for
// ambiguous errors where we can't be certain the request didn't succeed
// (network errors, 5xx server errors, non-ErrorResponse errors).
func isConfirmedRejection(err error) bool {
	var ghErr *github.ErrorResponse
	if !errors.As(err, &ghErr) {
		// Not a structured GitHub API error — could be network timeout,
		// DNS failure, etc. Treat as ambiguous.
		return false
	}
	// 4xx client errors (except 429 rate limit which might succeed on
	// retry) are definitive rejections; 5xx server errors are ambiguous.
	code := ghErr.Response.StatusCode
	return code >= http.StatusBadRequest && code < http.StatusInternalServerError && code != http.StatusTooManyRequests
}

// isNonRetryableRun returns true when GitHub's error message indicates the
// run itself can never be retried again (e.g. a rerun is already in
// progress, or the run has no failed jobs left) — as opposed to a rejection
// that might succeed on a later attempt. Detected by substring match on the
// documented error message since go-github exposes no typed error for this.
func isNonRetryableRun(err error) bool {
	var ghErr *github.ErrorResponse
	if !errors.As(err, &ghErr) {
		return false
	}
	return strings.Contains(strings.ToLower(ghErr.Message), "cannot be retried")
}

// escapeSlackMrkdwn escapes the three characters Slack's mrkdwn treats as
// markup, so untrusted/dynamic text (e.g. a GitHub API error message) can't
// corrupt the message or the trailing <url|label> link. Order matters: '&'
// must be escaped first so the entities added for '<'/'>' aren't themselves
// re-escaped.
func escapeSlackMrkdwn(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.ReplaceAll(s, ">", "&gt;")
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
