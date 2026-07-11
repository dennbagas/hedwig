package prcreate

import (
	"context"
	"fmt"

	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
	"go.uber.org/zap"
)

const (
	callbackFeature = "pr"
	actionRepo      = "repo"
	actionConfirm   = "confirm"
	actionCancel    = "cancel"
)

func (h *Handler) handleStart(ctx context.Context, chatID int64, triggeredBy string) error {
	rows := make([][]telegrambot.Button, len(h.repos))
	for i, r := range h.repos {
		cb := telegrambot.EncodeCallback(callbackFeature, actionRepo, r.Owner+"/"+r.Name)
		rows[i] = []telegrambot.Button{{Text: r.Name, CallbackData: cb}}
	}
	cancelCB := telegrambot.EncodeCallback(callbackFeature, actionCancel, "0")
	rows = append(rows, []telegrambot.Button{{Text: "Cancel", CallbackData: cancelCB}})

	msgID, err := h.tg.SendMessage(ctx, chatID,
		fmt.Sprintf("Triggered by: %s\n\nChoose a repository for the new PR:", triggeredBy),
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

func (h *Handler) handleRepoSelected(ctx context.Context, session *storage.PRSession, ownerRepo string) error {
	session.Repo = ownerRepo
	session.Step = storage.PRStepEnterTitle
	if err := h.store.UpsertPRSession(ctx, *session); err != nil {
		return err
	}
	return h.tg.EditMessage(ctx, session.ChatID, session.MessageID,
		fmt.Sprintf("Triggered by: %s\n\nRepo: <b>%s</b>\n\nPlease type the PR title:", session.TriggerUser, ownerRepo),
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
		fmt.Sprintf("Triggered by: %s\n\nRepo: <b>%s</b>\nTitle: <b>%s</b>\n\nPlease type the PR description:", session.TriggerUser, session.Repo, title),
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
		session.TriggerUser, session.Repo, h.sourceBranch, h.targetBranch, session.PRTitle, session.PRMessage)
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
		text = fmt.Sprintf("Triggered by: %s\n\nFailed to create PR: %v", session.TriggerUser, err)
		h.logger.Error("create PR failed", zap.Error(err), zap.String("triggered_by", session.TriggerUser))
	} else {
		text = fmt.Sprintf("Triggered by: %s\n\nPR created: <a href=\"%s\">%s</a>", session.TriggerUser, prURL, session.PRTitle)
		session.Status = storage.PRStatusCompleted
		h.logger.Info("PR created", zap.String("repo", session.Repo), zap.String("title", session.PRTitle), zap.String("triggered_by", session.TriggerUser), zap.String("url", prURL))
	}
	session.Step = storage.PRStepDone
	_ = h.store.UpsertPRSession(ctx, *session)
	return h.tg.EditMessage(ctx, session.ChatID, session.MessageID, text, telegrambot.WithParseMode("HTML"))
}

func (h *Handler) handleCancelled(ctx context.Context, session *storage.PRSession) error {
	session.Step = storage.PRStepCancelled
	session.Status = storage.PRStatusCancelled
	_ = h.store.UpsertPRSession(ctx, *session)
	h.logger.Info("PR creation cancelled", zap.String("repo", session.Repo), zap.String("triggered_by", session.TriggerUser))
	return h.tg.EditMessage(ctx, session.ChatID, session.MessageID,
		fmt.Sprintf("Triggered by: %s\n\nPR creation cancelled.", session.TriggerUser))
}

func splitOwnerRepo(ownerRepo string) (owner, repo string) {
	for i, c := range ownerRepo {
		if c == '/' {
			return ownerRepo[:i], ownerRepo[i+1:]
		}
	}
	return ownerRepo, ""
}
