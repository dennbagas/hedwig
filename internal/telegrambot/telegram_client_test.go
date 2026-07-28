package telegrambot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-telegram/bot"
)

// newTestClient starts a fake Telegram Bot API server driven by handler and
// returns a telegramClient pointed at it. The server (and the underlying
// bot's background goroutines) are cleaned up automatically.
func newTestClient(t *testing.T, handler http.HandlerFunc) *telegramClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	b, err := bot.New("test-token", bot.WithServerURL(server.URL), bot.WithSkipGetMe())
	if err != nil {
		t.Fatalf("bot.New() error = %v", err)
	}
	return &telegramClient{b: b}
}

func writeAPIResult(w http.ResponseWriter, result string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"ok":true,"result":`+result+`}`)
}

func TestTelegramClientSendMessage(t *testing.T) {
	var gotPath string
	var gotChatID, gotText, gotParseMode string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotChatID = r.FormValue("chat_id")
		gotText = r.FormValue("text")
		gotParseMode = r.FormValue("parse_mode")
		writeAPIResult(w, `{"message_id":42,"date":0,"chat":{"id":123,"type":"private"}}`)
	})

	id, err := client.SendMessage(context.Background(), 123, "hello world")
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if id != 42 {
		t.Errorf("SendMessage() id = %d, want 42", id)
	}
	if !strings.HasSuffix(gotPath, "/sendMessage") {
		t.Errorf("request path = %q, want suffix /sendMessage", gotPath)
	}
	if gotChatID != "123" {
		t.Errorf("chat_id = %q, want 123", gotChatID)
	}
	if gotText != "hello world" {
		t.Errorf("text = %q, want %q", gotText, "hello world")
	}
	if gotParseMode != "HTML" {
		t.Errorf("parse_mode = %q, want HTML (default)", gotParseMode)
	}
}

func TestTelegramClientSendMessageWithKeyboard(t *testing.T) {
	var gotReplyMarkup string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotReplyMarkup = r.FormValue("reply_markup")
		writeAPIResult(w, `{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}`)
	})

	_, err := client.SendMessage(context.Background(), 1, "pick one", WithInlineKeyboard([][]Button{
		{{Text: "Retry", CallbackData: "hedwig:retry:trigger:1"}},
	}))
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if !strings.Contains(gotReplyMarkup, "hedwig:retry:trigger:1") {
		t.Errorf("reply_markup = %q, want it to contain the callback data", gotReplyMarkup)
	}
}

func TestTelegramClientEditMessage(t *testing.T) {
	var gotPath, gotMessageID, gotText string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotMessageID = r.FormValue("message_id")
		gotText = r.FormValue("text")
		writeAPIResult(w, `{"message_id":7,"date":0,"chat":{"id":1,"type":"private"}}`)
	})

	err := client.EditMessage(context.Background(), 1, 7, "updated text")
	if err != nil {
		t.Fatalf("EditMessage() error = %v", err)
	}
	if !strings.HasSuffix(gotPath, "/editMessageText") {
		t.Errorf("request path = %q, want suffix /editMessageText", gotPath)
	}
	if gotMessageID != "7" {
		t.Errorf("message_id = %q, want 7", gotMessageID)
	}
	if gotText != "updated text" {
		t.Errorf("text = %q, want %q", gotText, "updated text")
	}
}

func TestTelegramClientEditMessageOmitsReplyMarkupByDefault(t *testing.T) {
	var gotReplyMarkup string
	var sawReplyMarkup bool
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotReplyMarkup, sawReplyMarkup = r.FormValue("reply_markup"), r.Form.Has("reply_markup")
		writeAPIResult(w, `{"message_id":7,"date":0,"chat":{"id":1,"type":"private"}}`)
	})

	if err := client.EditMessage(context.Background(), 1, 7, "updated text"); err != nil {
		t.Fatalf("EditMessage() error = %v", err)
	}
	if sawReplyMarkup {
		t.Errorf("reply_markup = %q, want no reply_markup field when no keyboard option is given", gotReplyMarkup)
	}
}

func TestTelegramClientEditMessageWithEmptyKeyboardRemovesButton(t *testing.T) {
	var gotReplyMarkup string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotReplyMarkup = r.FormValue("reply_markup")
		writeAPIResult(w, `{"message_id":7,"date":0,"chat":{"id":1,"type":"private"}}`)
	})

	err := client.EditMessage(context.Background(), 1, 7, "updated text", WithInlineKeyboard([][]Button{}))
	if err != nil {
		t.Fatalf("EditMessage() error = %v", err)
	}
	if gotReplyMarkup != `{"inline_keyboard":[]}` {
		t.Errorf("reply_markup = %q, want an empty inline keyboard so the button is removed", gotReplyMarkup)
	}
}

func TestTelegramClientEditMessageWithKeyboard(t *testing.T) {
	var gotReplyMarkup string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotReplyMarkup = r.FormValue("reply_markup")
		writeAPIResult(w, `{"message_id":7,"date":0,"chat":{"id":1,"type":"private"}}`)
	})

	err := client.EditMessage(context.Background(), 1, 7, "updated text", WithInlineKeyboard([][]Button{
		{{Text: "Retry", CallbackData: "hedwig:retry:trigger:1"}},
	}))
	if err != nil {
		t.Fatalf("EditMessage() error = %v", err)
	}
	if !strings.Contains(gotReplyMarkup, "hedwig:retry:trigger:1") {
		t.Errorf("reply_markup = %q, want it to contain the callback data", gotReplyMarkup)
	}
}

func TestTelegramClientRemoveKeyboard(t *testing.T) {
	var gotPath, gotReplyMarkup string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotReplyMarkup = r.FormValue("reply_markup")
		writeAPIResult(w, `{"message_id":9,"date":0,"chat":{"id":1,"type":"private"}}`)
	})

	if err := client.RemoveKeyboard(context.Background(), 1, 9); err != nil {
		t.Fatalf("RemoveKeyboard() error = %v", err)
	}
	if !strings.HasSuffix(gotPath, "/editMessageReplyMarkup") {
		t.Errorf("request path = %q, want suffix /editMessageReplyMarkup", gotPath)
	}
	if gotReplyMarkup != `{"inline_keyboard":[]}` {
		t.Errorf("reply_markup = %q, want an empty inline keyboard", gotReplyMarkup)
	}
}

func TestTelegramClientAnswerCallback(t *testing.T) {
	var gotPath, gotID, gotText string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotID = r.FormValue("callback_query_id")
		gotText = r.FormValue("text")
		writeAPIResult(w, `true`)
	})

	if err := client.AnswerCallback(context.Background(), "cbq-1", "done"); err != nil {
		t.Fatalf("AnswerCallback() error = %v", err)
	}
	if !strings.HasSuffix(gotPath, "/answerCallbackQuery") {
		t.Errorf("request path = %q, want suffix /answerCallbackQuery", gotPath)
	}
	if gotID != "cbq-1" || gotText != "done" {
		t.Errorf("got (id=%q, text=%q), want (cbq-1, done)", gotID, gotText)
	}
}

func TestTelegramClientSetWebhook(t *testing.T) {
	var gotPath, gotURL, gotSecret string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		gotURL = r.FormValue("url")
		gotSecret = r.FormValue("secret_token")
		writeAPIResult(w, `true`)
	})

	if err := client.SetWebhook(context.Background(), "https://example.com/hook", "shh"); err != nil {
		t.Fatalf("SetWebhook() error = %v", err)
	}
	if !strings.HasSuffix(gotPath, "/setWebhook") {
		t.Errorf("request path = %q, want suffix /setWebhook", gotPath)
	}
	if gotURL != "https://example.com/hook" || gotSecret != "shh" {
		t.Errorf("got (url=%q, secret=%q), want (https://example.com/hook, shh)", gotURL, gotSecret)
	}
}

func TestTelegramClientAPIErrorSurfaces(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"chat not found"}`)
	})

	_, err := client.SendMessage(context.Background(), 1, "hi")
	if err == nil {
		t.Fatal("SendMessage() error = nil, want an error surfaced from the Telegram API response")
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("SendMessage() error = %q, want it to include the API description", err.Error())
	}
}
