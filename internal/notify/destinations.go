package notify

import (
	"context"
	"errors"
	"fmt"

	"hedwig/internal/slackbot"
	"hedwig/internal/telegrambot"
)

// destinations holds where a notify handler delivers a rendered
// notification. Handlers embed this instead of separate tg/chatID/slack
// fields. Either client may be nil (that platform disabled).
type destinations struct {
	tg          telegrambot.Client
	chatID      int64
	slack       slackbot.Client
	slackChanID string
}

// send delivers telegramText to Telegram and slackText to Slack, whichever
// are enabled (client non-nil) and non-empty (that platform's template
// skipped the event, e.g. via an {{if}} that produced no output). Both
// sends are attempted even if one fails, so one platform's outage never
// blocks the other's delivery; any errors are joined so Dispatch's existing
// "handle event failed" logging still surfaces them.
func (d destinations) send(ctx context.Context, telegramText, slackText string) error {
	var errs []error
	if d.tg != nil && telegramText != "" {
		if _, err := d.tg.SendMessage(ctx, d.chatID, telegramText, telegrambot.WithParseMode("HTML")); err != nil {
			errs = append(errs, fmt.Errorf("send telegram: %w", err))
		}
	}
	if d.slack != nil && slackText != "" {
		if _, err := d.slack.PostMessage(ctx, d.slackChanID, slackbot.FormatQuoted(slackText), nil); err != nil {
			errs = append(errs, fmt.Errorf("send slack: %w", err))
		}
	}
	return errors.Join(errs...)
}
