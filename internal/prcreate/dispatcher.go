package prcreate

import (
	"context"
	"fmt"

	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
)

// HandleCommand handles the /newpr Telegram command.
func (h *Handler) HandleCommand(ctx context.Context, chatID int64) error {
	return h.handleStart(ctx, chatID)
}

// HandleCallback routes an inline keyboard button tap for the PR creation flow.
func (h *Handler) HandleCallback(ctx context.Context, callbackQueryID string, chatID, messageID int64, action, payload string) error {
	_ = h.tg.AnswerCallback(ctx, callbackQueryID, "")

	if action == actionCancel {
		session, err := h.store.GetPRSession(ctx, chatID, messageID)
		if err != nil {
			return fmt.Errorf("get pr session: %w", err)
		}
		if session == nil {
			return nil
		}
		return h.handleCancelled(ctx, session)
	}

	if action == actionRepo {
		session, err := h.store.GetActivePRSession(ctx, chatID)
		if err != nil {
			return fmt.Errorf("get active pr session: %w", err)
		}
		if session == nil {
			return nil
		}
		return h.handleRepoSelected(ctx, session, payload)
	}

	if action == actionSkip {
		session, err := h.store.GetActivePRSession(ctx, chatID)
		if err != nil {
			return fmt.Errorf("get active pr session: %w", err)
		}
		if session == nil || session.Step != storage.PRStepEnterMessage {
			return nil
		}
		return h.handleMessageEntered(ctx, session, "")
	}

	if action == actionConfirm {
		session, err := h.store.GetPRSession(ctx, chatID, messageID)
		if err != nil {
			return fmt.Errorf("get pr session: %w", err)
		}
		if session == nil || session.Step != storage.PRStepConfirm {
			return nil
		}
		return h.handleConfirmed(ctx, session)
	}

	return nil
}

// HandleTextMessage routes free-text messages to the active PR session step.
func (h *Handler) HandleTextMessage(ctx context.Context, chatID int64, text string) error {
	session, err := h.store.GetActivePRSession(ctx, chatID)
	if err != nil {
		return fmt.Errorf("get active pr session: %w", err)
	}
	if session == nil {
		return nil
	}

	switch session.Step {
	case storage.PRStepEnterTitle:
		return h.handleTitleEntered(ctx, session, text)
	case storage.PRStepEnterMessage:
		return h.handleMessageEntered(ctx, session, text)
	default:
		// No action expected for other steps from a plain text message.
		return nil
	}
}

// IsFeatureCallback reports whether the callback data belongs to the prcreate feature.
func IsFeatureCallback(data string) bool {
	feature, _, _, err := telegrambot.DecodeCallback(data)
	return err == nil && feature == callbackFeature
}
