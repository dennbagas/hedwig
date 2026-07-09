package telegrambot

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type telegramClient struct {
	b *bot.Bot
}

func New(token string) (Client, error) {
	b, err := bot.New(token)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	return &telegramClient{b: b}, nil
}

func (c *telegramClient) SendMessage(ctx context.Context, chatID int64, text string, opts ...SendOption) (int64, error) {
	p := applyOpts(opts)
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseMode(p.parseMode),
	}
	if len(p.keyboard) > 0 {
		params.ReplyMarkup = buildKeyboard(p.keyboard)
	}
	msg, err := c.b.SendMessage(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("send message: %w", err)
	}
	return int64(msg.ID), nil
}

func (c *telegramClient) EditMessage(ctx context.Context, chatID, messageID int64, text string, opts ...SendOption) error {
	p := applyOpts(opts)
	params := &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: int(messageID),
		Text:      text,
		ParseMode: models.ParseMode(p.parseMode),
	}
	if len(p.keyboard) > 0 {
		params.ReplyMarkup = buildKeyboard(p.keyboard)
	}
	_, err := c.b.EditMessageText(ctx, params)
	return err
}

func (c *telegramClient) RemoveKeyboard(ctx context.Context, chatID, messageID int64) error {
	_, err := c.b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:      chatID,
		MessageID:   int(messageID),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}},
	})
	return err
}

func (c *telegramClient) AnswerCallback(ctx context.Context, callbackQueryID, text string) error {
	_, err := c.b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callbackQueryID,
		Text:            text,
	})
	return err
}

func (c *telegramClient) SetWebhook(ctx context.Context, webhookURL, secretToken string) error {
	_, err := c.b.SetWebhook(ctx, &bot.SetWebhookParams{
		URL:         webhookURL,
		SecretToken: secretToken,
	})
	return err
}

func applyOpts(opts []SendOption) *sendParams {
	p := &sendParams{parseMode: string(models.ParseModeHTML)}
	for _, o := range opts {
		o(p)
	}
	return p
}

func buildKeyboard(rows [][]Button) *models.InlineKeyboardMarkup {
	kb := make([][]models.InlineKeyboardButton, len(rows))
	for i, row := range rows {
		kb[i] = make([]models.InlineKeyboardButton, len(row))
		for j, btn := range row {
			kb[i][j] = models.InlineKeyboardButton{
				Text:         btn.Text,
				CallbackData: btn.CallbackData,
			}
		}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: kb}
}
