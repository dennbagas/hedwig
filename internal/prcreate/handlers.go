package prcreate

import (
	"context"
	"fmt"
	"html"

	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
	"go.uber.org/zap"
)

// esc escapes text so it is safe to embed in a Telegram ParseMode=HTML message.
func esc(s string) string {
	return html.EscapeString(s)
}

const (
	callbackFeature = "pr"
	actionRepo      = "repo"
	actionConfirm   = "confirm"
	actionCancel    = "cancel"
)

func (h *Handler) handleStart(ctx context.Context, chatID int64, triggeredBy string) error {
	if err := h.cancelActiveSession(ctx, chatID); err != nil {
		return fmt.Errorf("cancel previous pr session: %w", err)
	}

	rows := make([][]telegrambot.Button, len(h.repos))
	for i, r := range h.repos {
		cb := telegrambot.EncodeCallback(callbackFeature, actionRepo, r.Owner+"/"+r.Name)
		rows[i] = []telegrambot.Button{{Text: r.Name, CallbackData: cb}}
	}
	cancelCB := telegrambot.EncodeCallback(callbackFeature, actionCancel, "0")
	rows = append(rows, []telegrambot.Button{{Text: "Cancel", CallbackData: cancelCB}})

	msgID, err := h.tg.SendMessage(ctx, chatID,
		fmt.Sprintf("Triggered by: %s\n\nChoose a repository for the new PR:", esc(triggeredBy)),
		telegrambot.WithInlineKeyboard(rows))
	if err != nil {
		return fmt.Errorf("send repo selection: %w", err)
	}

	return h.store.UpsertPRSession(ctx, storage.PRSession{
		ChatID:      chatID,
		MessageID:   msgID,
		Step:        storage.PRStepSelectRepo,
		Status:      storage.PRStatusInProgress,
		TriggerUser: triggeredBy,
	})
}

// cancelActiveSession supersedes any in-progress PR session for chatID so at
// most one session is ever "active" per chat, keeping GetActivePRSession
// unambiguous for callback/reply routing.
func (h *Handler) cancelActiveSession(ctx context.Context, chatID int64) error {
	session, err := h.store.GetActivePRSession(ctx, chatID)
	if err != nil {
		return fmt.Errorf("get active pr session: %w", err)
	}
	if session == nil {
		return nil
	}
	session.Step = storage.PRStepCancelled
	session.Status = storage.PRStatusCancelled
	if err := h.store.UpsertPRSession(ctx, *session); err != nil {
		return err
	}
	if err := h.tg.RemoveKeyboard(ctx, session.ChatID, session.MessageID); err != nil {
		h.logger.Warn("remove keyboard for superseded pr session", zap.Error(err))
	}
	return nil
}

func (h *Handler) handleRepoSelected(ctx context.Context, session *storage.PRSession, ownerRepo string) error {
	session.Repo = ownerRepo
	session.Step = storage.PRStepEnterTitle
	if err := h.store.UpsertPRSession(ctx, *session); err != nil {
		return err
	}
	return h.tg.EditMessage(ctx, session.ChatID, session.MessageID,
		fmt.Sprintf("Triggered by: %s\n\nRepo: <b>%s</b>\n\nPlease type the PR title:", esc(session.TriggerUser), esc(ownerRepo)),
		telegrambot.WithParseMode("HTML"))
}

func (h *Handler) handleTitleEntered(ctx context.Context, session *storage.PRSession, title string) error {
	session.PRTitle = title
	session.Step = storage.PRStepEnterMessage
	if err := h.store.UpsertPRSession(ctx, *session); err != nil {
		return err
	}
	cancelCB := telegrambot.EncodeCallback(callbackFeature, actionCancel, "0")
	return h.tg.EditMessage(ctx, session.ChatID, session.MessageID,
		fmt.Sprintf("Triggered by: %s\n\nRepo: <b>%s</b>\nTitle: <b>%s</b>\n\nPlease type the PR description:", esc(session.TriggerUser), esc(session.Repo), esc(title)),
		telegrambot.WithParseMode("HTML"),
		telegrambot.WithInlineKeyboard([][]telegrambot.Button{
			{{Text: "Cancel", CallbackData: cancelCB}},
		}))
}

func (h *Handler) handleMessageEntered(ctx context.Context, session *storage.PRSession, body string) error {
	session.PRMessage = body
	session.Step = storage.PRStepConfirm
	if err := h.store.UpsertPRSession(ctx, *session); err != nil {
		return err
	}
	confirmCB := telegrambot.EncodeCallback(callbackFeature, actionConfirm, "1")
	cancelCB := telegrambot.EncodeCallback(callbackFeature, actionCancel, "0")
	summary := fmt.Sprintf(
		"Triggered by: %s\n\n<b>Summary</b>\nRepo: %s\nSource → Target: %s → %s\nTitle: %s\n\nDescription:\n%s",
		esc(session.TriggerUser), esc(session.Repo), esc(h.sourceBranch), esc(h.targetBranch), esc(session.PRTitle), esc(session.PRMessage))
	return h.tg.EditMessage(ctx, session.ChatID, session.MessageID,
		summary,
		telegrambot.WithParseMode("HTML"),
		telegrambot.WithInlineKeyboard([][]telegrambot.Button{
			{
				{Text: "Confirm", CallbackData: confirmCB},
				{Text: "Cancel", CallbackData: cancelCB},
			},
		}))
}

func (h *Handler) handleConfirmed(ctx context.Context, session *storage.PRSession, triggeredBy string) error {
	owner, repo := splitOwnerRepo(session.Repo)
	prURL, err := h.github.CreatePR(ctx, owner, repo,
		session.PRTitle, session.PRMessage, h.sourceBranch, h.targetBranch)
	var text string
	if err != nil {
		text = fmt.Sprintf("Triggered by: %s\n\nFailed to create PR: %s", esc(session.TriggerUser), esc(err.Error()))
		h.logger.Error("create PR failed", zap.Error(err), zap.String("triggered_by", session.TriggerUser))
	} else {
		text = fmt.Sprintf("Triggered by: %s\n\nPR created: <a href=\"%s\">%s</a>", esc(session.TriggerUser), esc(prURL), esc(session.PRTitle))
		session.Status = storage.PRStatusCompleted
	}
	session.Step = storage.PRStepDone
	_ = h.store.UpsertPRSession(ctx, *session)
	return h.tg.EditMessage(ctx, session.ChatID, session.MessageID, text, telegrambot.WithParseMode("HTML"))
}

func (h *Handler) handleCancelled(ctx context.Context, session *storage.PRSession) error {
	session.Step = storage.PRStepCancelled
	session.Status = storage.PRStatusCancelled
	_ = h.store.UpsertPRSession(ctx, *session)
	return h.tg.EditMessage(ctx, session.ChatID, session.MessageID,
		fmt.Sprintf("Triggered by: %s\n\nPR creation cancelled.", esc(session.TriggerUser)))
}

func splitOwnerRepo(ownerRepo string) (owner, repo string) {
	for i, c := range ownerRepo {
		if c == '/' {
			return ownerRepo[:i], ownerRepo[i+1:]
		}
	}
	return ownerRepo, ""
}
