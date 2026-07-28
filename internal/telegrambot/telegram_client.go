package telegrambot

import (
	"context"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type telegramClient struct {
	b *bot.Bot
}

func New(token string) (Client, error) {
	var b *bot.Bot
	var err error
	delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	for i, d := range delays {
		b, err = bot.New(token)
		if err == nil {
			return &telegramClient{b: b}, nil
		}
		if i < len(delays)-1 {
			time.Sleep(d)
		}
	}
	return nil, fmt.Errorf("create telegram bot: %w", err)
}

func (c *telegramClient) SendMessage(ctx context.Context, chatID int64, text string, opts ...SendOption) (int64, error) {
	p := ApplyOptions(opts...)
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseMode(p.ParseMode),
	}
	if len(p.Keyboard) > 0 {
		params.ReplyMarkup = buildKeyboard(p.Keyboard)
	}
	msg, err := c.b.SendMessage(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("send message: %w", err)
	}
	return int64(msg.ID), nil
}

func (c *telegramClient) EditMessage(ctx context.Context, chatID, messageID int64, text string, opts ...SendOption) error {
	p := ApplyOptions(opts...)
	params := &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: int(messageID),
		Text:      text,
		ParseMode: models.ParseMode(p.ParseMode),
	}
	if p.Keyboard != nil {
		params.ReplyMarkup = buildKeyboard(p.Keyboard)
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
