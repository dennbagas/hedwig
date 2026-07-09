package telegrambot

import (
	"context"
)

type Button struct {
	Text         string
	CallbackData string
}

type SendOption func(*sendParams)

type sendParams struct {
	keyboard  [][]Button
	parseMode string
}

func WithInlineKeyboard(rows [][]Button) SendOption {
	return func(p *sendParams) { p.keyboard = rows }
}

func WithParseMode(mode string) SendOption {
	return func(p *sendParams) { p.parseMode = mode }
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
