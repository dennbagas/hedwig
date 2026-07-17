package notify

import (
	"context"
	"fmt"

	"hedwig/internal/retry"
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

func newDispatcher(tg telegrambot.Client, chatID int64, logger zerolog.Logger) *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]EventHandler),
		tg:       tg,
		chatID:   chatID,
		logger:   logger,
	}
}

// New creates a Dispatcher with all event handlers registered and templates
// loaded from templatesDir. Returns an error if any template file fails to parse.
func New(tg telegrambot.Client, chatID int64, retryH *retry.Handler, templatesDir string, logger zerolog.Logger) (*Dispatcher, error) {
	templateLoader, err := newTemplateLoader(templatesDir, logger)
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	d := newDispatcher(tg, chatID, logger)
	registerAll(d, tg, retryH, chatID, templateLoader)
	return d, nil
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

func (d *Dispatcher) Register(eventType string, h EventHandler) {
	d.handlers[eventType] = h
}

func registerAll(d *Dispatcher, tg telegrambot.Client, retryH *retry.Handler, chatID int64, l *templateLoader) {
	d.Register("push", &pushHandler{tg: tg, chatID: chatID, loader: l})
	d.Register("pull_request", &pullRequestHandler{tg: tg, chatID: chatID, loader: l})
	d.Register("create", &createHandler{tg: tg, chatID: chatID, loader: l})
	d.Register("issue_comment", &issueCommentHandler{tg: tg, chatID: chatID, loader: l})
	d.Register("pull_request_review", &pullRequestReviewHandler{tg: tg, chatID: chatID, loader: l})
	d.Register("pull_request_review_comment", &pullRequestReviewCommentHandler{tg: tg, chatID: chatID, loader: l})
	d.Register("workflow_run", &workflowRunHandler{tg: tg, chatID: chatID, retryH: retryH, loader: l})
	d.Register("release", &releaseHandler{tg: tg, chatID: chatID, loader: l})
}
