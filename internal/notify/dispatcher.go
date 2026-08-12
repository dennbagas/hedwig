package notify

import (
	"context"
	"fmt"

	"hedwig/internal/retry"
	"hedwig/internal/slackbot"
	"hedwig/internal/telegrambot"

	"github.com/rs/zerolog"
)

type EventHandler interface {
	Handle(ctx context.Context, event any) error
}

type Dispatcher struct {
	handlers map[string]EventHandler
	logger   zerolog.Logger
}

func newDispatcher(logger zerolog.Logger) *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]EventHandler),
		logger:   logger,
	}
}

// New creates a Dispatcher with all event handlers registered and templates
// loaded from templatesDir. Returns an error if any template file fails to
// parse. tg/slack may be nil when that platform is disabled — every event
// handler skips a nil platform's send.
func New(tg telegrambot.Client, chatID int64, slack slackbot.Client, slackChanID string, retryH *retry.Handler, retryEnabled bool, templatesDir string, logger zerolog.Logger) (*Dispatcher, error) {
	templateLoader, err := newTemplateLoader(templatesDir, logger)
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	d := newDispatcher(logger)
	dest := destinations{tg: tg, chatID: chatID, slack: slack, slackChanID: slackChanID}
	registerAll(d, dest, retryH, retryEnabled, templateLoader)
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

func registerAll(d *Dispatcher, dest destinations, retryH *retry.Handler, retryEnabled bool, l *templateLoader) {
	d.Register("push", &pushHandler{destinations: dest, loader: l})
	d.Register("pull_request", &pullRequestHandler{destinations: dest, loader: l})
	d.Register("create", &createHandler{destinations: dest, loader: l})
	d.Register("issue_comment", &issueCommentHandler{destinations: dest, loader: l})
	d.Register("pull_request_review", &pullRequestReviewHandler{destinations: dest, loader: l})
	d.Register("pull_request_review_comment", &pullRequestReviewCommentHandler{destinations: dest, loader: l})
	d.Register("workflow_run", &workflowRunHandler{destinations: dest, retryH: retryH, retryDisabled: !retryEnabled, loader: l})
	d.Register("release", &releaseHandler{destinations: dest, loader: l})
}
