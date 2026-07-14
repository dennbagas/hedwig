package telegrambot

import (
	"context"
)

type Button struct {
	Text         string
	CallbackData string
}

type SendOption func(*SendParams)

// SendParams is the decoded result of applying a set of SendOptions.
// Exported so callers (including test fakes) can inspect what a
// SendMessage/EditMessage call requested without needing access to
// Telegram Bot API internals.
type SendParams struct {
	Keyboard  [][]Button
	ParseMode string
}

func WithInlineKeyboard(rows [][]Button) SendOption {
	return func(p *SendParams) { p.Keyboard = rows }
}

func WithParseMode(mode string) SendOption {
	return func(p *SendParams) { p.ParseMode = mode }
}

// ApplyOptions applies opts to a fresh SendParams, defaulting ParseMode to
// "HTML" (matching the default every real send/edit call uses).
func ApplyOptions(opts ...SendOption) SendParams {
	p := SendParams{ParseMode: "HTML"}
	for _, o := range opts {
		o(&p)
	}
	return p
}

type Client interface {
	// SendMessage sends a message to chatID and returns the message ID.
	SendMessage(ctx context.Context, chatID int64, text string, opts ...SendOption) (int64, error)
	// EditMessage edits an existing message in place.
	EditMessage(ctx context.Context, chatID, messageID int64, text string, opts ...SendOption) error
	// RemoveKeyboard strips the inline keyboard from an existing message.
	RemoveKeyboard(ctx context.Context, chatID, messageID int64) error
	// AnswerCallback acknowledges a callback query.
	AnswerCallback(ctx context.Context, callbackQueryID, text string) error
	// SetWebhook registers the bot's webhook URL with Telegram.
	SetWebhook(ctx context.Context, webhookURL, secretToken string) error
}
