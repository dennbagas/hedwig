package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
)

func TestTelegramWebhookWrongSecret(t *testing.T) {
	ts := newTestServer(t, []int64{111}, "correct-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{}`))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-secret")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestTelegramWebhookCorrectSecretAccepted(t *testing.T) {
	ts := newTestServer(t, nil, "correct-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{}`))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "correct-secret")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for the correct secret", rec.Code)
	}
}

func TestTelegramWebhookDisallowedUser(t *testing.T) {
	ts := newTestServer(t, []int64{111}, "secret")

	body := `{"update_id":1,"message":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"},"from":{"id":999,"is_bot":false},"text":"hi"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(ts.tg.Sent) != 0 {
		t.Error("expected no processing for a disallowed user")
	}
}

func TestTelegramWebhookRetryCallbackRoutesToRetryHandler(t *testing.T) {
	ts := newTestServer(t, []int64{111}, "secret")
	ctx := context.Background()

	id, err := ts.store.CreateRetry(ctx, storage.CICDRetry{ChatID: 1, MessageID: 2, RunID: 55, Repo: "acme/widgets", Status: storage.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}

	cb := telegrambot.EncodeCallback("retry", "trigger", strconv.FormatInt(id, 10))
	body := fmt.Sprintf(`{"update_id":1,"callback_query":{"id":"cbq-1","from":{"id":111,"is_bot":false},"message":{"message_id":2,"date":0,"chat":{"id":1,"type":"private"}},"data":%q}}`, cb)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(ts.gh.RerunCalls) != 1 {
		t.Fatalf("RerunCalls = %+v, want exactly one (retry.Handler should have run)", ts.gh.RerunCalls)
	}
	if len(ts.tg.AnsweredCallbacks) != 1 || ts.tg.AnsweredCallbacks[0].CallbackQueryID != "cbq-1" {
		t.Errorf("AnsweredCallbacks = %+v, want the callback query answered", ts.tg.AnsweredCallbacks)
	}
}

func TestTelegramWebhookMalformedCallbackData(t *testing.T) {
	ts := newTestServer(t, []int64{111}, "secret")

	body := `{"update_id":1,"callback_query":{"id":"cbq-1","from":{"id":111,"is_bot":false},"message":{"message_id":2,"date":0,"chat":{"id":1,"type":"private"}},"data":"not-a-valid-format"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (malformed callback data is ignored, not fatal)", rec.Code)
	}
}

func TestTelegramWebhookUnknownFeature(t *testing.T) {
	ts := newTestServer(t, []int64{111}, "secret")

	cb := telegrambot.EncodeCallback("mystery-feature", "action", "1")
	body := fmt.Sprintf(`{"update_id":1,"callback_query":{"id":"cbq-1","from":{"id":111,"is_bot":false},"message":{"message_id":2,"date":0,"chat":{"id":1,"type":"private"}},"data":%q}}`, cb)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(ts.gh.RerunCalls) != 0 {
		t.Error("expected no GitHub calls for an unknown callback feature")
	}
}

func TestTelegramWebhookPlainTextMessageIsNoop(t *testing.T) {
	ts := newTestServer(t, []int64{111}, "secret")

	body := `{"update_id":1,"message":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"},"from":{"id":111,"is_bot":false},"text":"hello bot"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(ts.tg.Sent) != 0 {
		t.Error("expected no message sent for a plain-text update (no /newpr wizard anymore)")
	}
}
