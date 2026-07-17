package notify

import (
	"context"
	"fmt"

	"hedwig/internal/telegrambot"

	"github.com/rs/zerolog"
)

type EventHandler interface {
	Handle(ctx context.Context, event any) error
}

type Dispatcher struct {
	handlers map[string]EventHandler
	tg       telegrambot.Client
	chatID   int64
	logger   zerolog.Logger
}

func NewDispatcher(tg telegrambot.Client, chatID int64, logger zerolog.Logger) *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]EventHandler),
		tg:       tg,
		chatID:   chatID,
		logger:   logger,
	}
}

func (d *Dispatcher) Register(eventType string, h EventHandler) {
	d.handlers[eventType] = h
}

func (d *Dispatcher) Dispatch(ctx context.Context, eventType string, event any) error {
	h, ok := d.handlers[eventType]
	if !ok {
		d.logger.Debug().Str("event_type", eventType).Msg("no handler registered for event type")
		return nil
	}
	d.logger.Info().Str("event_type", eventType).Msg("dispatching event")
	if err := h.Handle(ctx, event); err != nil {
		d.logger.Error().Str("event_type", eventType).Err(err).Msg("handle event failed")
		return fmt.Errorf("handle event %s: %w", eventType, err)
	}
	return nil
}
