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
	"go.uber.org/zap"
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
		logger.Error("read telegram webhook body", zap.Error(err))
		w.WriteHeader(http.StatusOK)
		return
	}

	var update models.Update
	if err := json.Unmarshal(body, &update); err != nil {
		logger.Error("unmarshal telegram update", zap.Error(err))
		w.WriteHeader(http.StatusOK)
		return
	}

	if !telegrambot.IsAllowed(s.allowedUserIDs, &update) {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch {
	case update.CallbackQuery != nil:
		if err := s.routeCallback(ctx, logger, update.CallbackQuery); err != nil {
			logger.Error("route callback", zap.Error(err))
		}
	case update.Message != nil:
		if err := s.routeMessage(ctx, logger, update.Message); err != nil {
			logger.Error("route message", zap.Error(err))
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) routeCallback(ctx context.Context, logger *zap.Logger, cq *models.CallbackQuery) error {
	data := cq.Data
	chatID, messageID := extractChatAndMessageID(&cq.Message)

	feature, _, payload, err := telegrambot.DecodeCallback(data)
	if err != nil {
		logger.Warn("decode callback data", zap.Error(err), zap.String("data", data))
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
		logger.Warn("unknown callback feature", zap.String("feature", feature))
	}
	return nil
}

func (s *Server) routeMessage(_ context.Context, logger *zap.Logger, msg *models.Message) error {
	logger.Debug("unhandled command", zap.String("text", msg.Text))
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
