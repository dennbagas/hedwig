package httpserver

import (
	"net/http"

	"hedwig/internal/logging"
)

func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.FromContext(ctx)

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	logger = logger.With().Str("delivery_id", deliveryID).Logger()

	payload, err := s.github.ValidateWebhook(r)
	if err != nil {
		logger.Warn().Err(err).Msg("invalid webhook signature")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	if deliveryID != "" {
		isDuplicate, err := s.store.RecordDelivery(ctx, deliveryID)
		if err != nil {
			logger.Error().Err(err).Msg("record delivery")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if isDuplicate {
			logger.Info().Msg("duplicate delivery, skipping")
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	eventType := r.Header.Get("X-GitHub-Event")
	event, err := s.github.ParseWebhook(eventType, payload)
	if err != nil {
		logger.Warn().Err(err).Str("event_type", eventType).Msg("parse webhook")
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := s.notifyD.Dispatch(ctx, eventType, event); err != nil {
		logger.Error().Err(err).Str("event_type", eventType).Msg("dispatch event")
		if deliveryID != "" {
			if delErr := s.store.DeleteDelivery(ctx, deliveryID); delErr != nil {
				logger.Error().Err(delErr).Msg("delete delivery record after dispatch failure")
			}
		}
		http.Error(w, "dispatch failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
