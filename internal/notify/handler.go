package notify

import (
	"context"
	"fmt"

	"github.com/btse/hedwig/internal/telegrambot"
	"go.uber.org/zap"
)

type EventHandler interface {
	Handle(ctx context.Context, event any) error
}

type Dispatcher struct {
	handlers map[string]EventHandler
	tg       telegrambot.Client
	chatID   int64
	logger   *zap.Logger
}

func NewDispatcher(tg telegrambot.Client, chatID int64, logger *zap.Logger) *Dispatcher {
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
		d.logger.Debug("no handler registered for event type", zap.String("event_type", eventType))
		return nil
	}
	d.logger.Info("dispatching event", zap.String("event_type", eventType))
	if err := h.Handle(ctx, event); err != nil {
		d.logger.Error("handle event failed", zap.String("event_type", eventType), zap.Error(err))
		return fmt.Errorf("handle event %s: %w", eventType, err)
	}
	return nil
}
