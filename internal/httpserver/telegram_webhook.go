package httpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"hedwig/internal/logging"
	"hedwig/internal/telegrambot"

	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog"
)

func (s *Server) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.FromContext(ctx)

	got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.telegramSecret)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error().Err(err).Msg("read telegram webhook body")
		w.WriteHeader(http.StatusOK)
		return
	}

	var update models.Update
	if err := json.Unmarshal(body, &update); err != nil {
		logger.Error().Err(err).Msg("unmarshal telegram update")
		w.WriteHeader(http.StatusOK)
		return
	}

	if !telegrambot.IsAllowed(s.allowedUserIDs, &update) {
		if update.CallbackQuery != nil {
			logger.Warn().
				Int64("user_id", update.CallbackQuery.From.ID).
				Str("username", update.CallbackQuery.From.Username).
				Msg("unauthorized user attempted callback")
			if err := s.tg.AnswerCallback(ctx, update.CallbackQuery.ID, "You're not authorized to use this."); err != nil {
				logger.Error().Err(err).Msg("answer unauthorized callback")
			}
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	switch {
	case update.CallbackQuery != nil:
		if err := s.routeCallback(ctx, logger, update.CallbackQuery); err != nil {
			logger.Error().Err(err).Msg("route callback")
		}
	case update.Message != nil:
		if err := s.routeMessage(ctx, logger, update.Message); err != nil {
			logger.Error().Err(err).Msg("route message")
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) routeCallback(ctx context.Context, logger zerolog.Logger, cq *models.CallbackQuery) error {
	data := cq.Data
	chatID, messageID := extractChatAndMessageID(&cq.Message)

	feature, _, payload, err := telegrambot.DecodeCallback(data)
	if err != nil {
		logger.Warn().Err(err).Str("data", data).Msg("decode callback data")
		return nil
	}

	switch feature {
	case "retry":
		retryID, err := strconv.ParseInt(payload, 10, 64)
		if err != nil {
			return nil
		}
		return s.retryH.HandleCallback(ctx, cq.ID, chatID, messageID, retryID)
	default:
		logger.Warn().Str("feature", feature).Msg("unknown callback feature")
	}
	return nil
}

func (s *Server) routeMessage(_ context.Context, logger zerolog.Logger, msg *models.Message) error {
	logger.Debug().Str("text", msg.Text).Msg("unhandled command")
	return nil
}

// extractChatAndMessageID returns the chat and message IDs from a MaybeInaccessibleMessage.
func extractChatAndMessageID(m *models.MaybeInaccessibleMessage) (chatID, messageID int64) {
	if m == nil {
		return 0, 0
	}
	if m.Message != nil {
		return m.Message.Chat.ID, int64(m.Message.ID)
	}
	if m.InaccessibleMessage != nil {
		return m.InaccessibleMessage.Chat.ID, int64(m.InaccessibleMessage.MessageID)
	}
	return 0, 0
}
