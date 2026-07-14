// Package telegrambottest provides a fake telegrambot.Client for use in
// tests, following the same companion-package convention as net/http/httptest.
package telegrambottest

import (
	"context"
	"sync"

	"github.com/btse/hedwig/internal/telegrambot"
)

// SentMessage records a single SendMessage or EditMessage call.
type SentMessage struct {
	ChatID    int64
	MessageID int64 // assigned by FakeClient for SendMessage; as given for EditMessage
	Text      string
	Params    telegrambot.SendParams
	Edited    bool
}

// RemovedKeyboard records a single RemoveKeyboard call.
type RemovedKeyboard struct {
	ChatID    int64
	MessageID int64
}

// AnsweredCallback records a single AnswerCallback call.
type AnsweredCallback struct {
	CallbackQueryID string
	Text            string
}

// SetWebhookCall records a single SetWebhook call.
type SetWebhookCall struct {
	WebhookURL  string
	SecretToken string
}

// FakeClient is an in-memory telegrambot.Client test double. Every call is
// recorded for assertions; set the *Err fields to make a method return an
// error instead of succeeding. Safe for concurrent use.
type FakeClient struct {
	mu sync.Mutex

	nextMessageID int64

	Sent              []SentMessage
	RemovedKeyboards  []RemovedKeyboard
	AnsweredCallbacks []AnsweredCallback
	Webhooks          []SetWebhookCall

	SendMessageErr    error
	EditMessageErr    error
	RemoveKeyboardErr error
	AnswerCallbackErr error
	SetWebhookErr     error
}

// New returns a ready-to-use FakeClient.
func New() *FakeClient {
	return &FakeClient{nextMessageID: 1}
}

func (f *FakeClient) SendMessage(_ context.Context, chatID int64, text string, opts ...telegrambot.SendOption) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SendMessageErr != nil {
		return 0, f.SendMessageErr
	}
	id := f.nextMessageID
	f.nextMessageID++
	f.Sent = append(f.Sent, SentMessage{
		ChatID:    chatID,
		MessageID: id,
		Text:      text,
		Params:    telegrambot.ApplyOptions(opts...),
	})
	return id, nil
}

func (f *FakeClient) EditMessage(_ context.Context, chatID, messageID int64, text string, opts ...telegrambot.SendOption) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.EditMessageErr != nil {
		return f.EditMessageErr
	}
	f.Sent = append(f.Sent, SentMessage{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
		Params:    telegrambot.ApplyOptions(opts...),
		Edited:    true,
	})
	return nil
}

func (f *FakeClient) RemoveKeyboard(_ context.Context, chatID, messageID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.RemoveKeyboardErr != nil {
		return f.RemoveKeyboardErr
	}
	f.RemovedKeyboards = append(f.RemovedKeyboards, RemovedKeyboard{ChatID: chatID, MessageID: messageID})
	return nil
}

func (f *FakeClient) AnswerCallback(_ context.Context, callbackQueryID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.AnswerCallbackErr != nil {
		return f.AnswerCallbackErr
	}
	f.AnsweredCallbacks = append(f.AnsweredCallbacks, AnsweredCallback{CallbackQueryID: callbackQueryID, Text: text})
	return nil
}

func (f *FakeClient) SetWebhook(_ context.Context, webhookURL, secretToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SetWebhookErr != nil {
		return f.SetWebhookErr
	}
	f.Webhooks = append(f.Webhooks, SetWebhookCall{WebhookURL: webhookURL, SecretToken: secretToken})
	return nil
}

var _ telegrambot.Client = (*FakeClient)(nil)
