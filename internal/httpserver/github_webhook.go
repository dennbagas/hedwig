package httpserver

import (
	"net/http"

	"github.com/btse/hedwig/internal/logging"
	"go.uber.org/zap"
)

func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.FromContext(ctx)

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	logger = logger.With(zap.String("delivery_id", deliveryID))

	payload, err := s.github.ValidateWebhook(r)
	if err != nil {
		logger.Warn("invalid webhook signature", zap.Error(err))
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	if deliveryID != "" {
		isDuplicate, err := s.store.RecordDelivery(ctx, deliveryID)
		if err != nil {
			logger.Error("record delivery", zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if isDuplicate {
			logger.Info("duplicate delivery, skipping")
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	eventType := r.Header.Get("X-GitHub-Event")
	event, err := s.github.ParseWebhook(eventType, payload)
	if err != nil {
		logger.Warn("parse webhook", zap.Error(err), zap.String("event_type", eventType))
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := s.notifyD.Dispatch(ctx, eventType, event); err != nil {
		logger.Error("dispatch event", zap.Error(err), zap.String("event_type", eventType))
	}

	w.WriteHeader(http.StatusOK)
}
