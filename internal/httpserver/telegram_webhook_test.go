package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"hedwig/internal/database"
	"hedwig/internal/telegrambot"
)

func TestTelegramWebhookWrongSecret(t *testing.T) {
	ts := newTestServer(t, "correct-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{}`))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-secret")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestTelegramWebhookCorrectSecretAccepted(t *testing.T) {
	ts := newTestServer(t, "correct-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(`{}`))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "correct-secret")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for the correct secret", rec.Code)
	}
}

func TestTelegramWebhookRetryCallbackRoutesToRetryHandler(t *testing.T) {
	ts := newTestServer(t, "secret")
	ctx := context.Background()

	id, err := ts.store.CreateRetry(ctx, database.CICDRetry{RunID: 55, Repo: "acme/widgets", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	if err := ts.store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformTelegram, ChatRef: "1", MessageRef: "2", MessageText: "msg"}); err != nil {
		t.Fatalf("CreateRetryTarget() error = %v", err)
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
	ts := newTestServer(t, "secret")

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
	ts := newTestServer(t, "secret")

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

// TestTelegramWebhookPlainTextMessageIsNoop covers messages from more than
// one sender: routeMessage is currently a no-op regardless of sender, since
// there is no allowlist gating messages (see docs/plans/phase-2-pr-creation.md
// for when a message-based command, and its gate, should return).
func TestTelegramWebhookPlainTextMessageIsNoop(t *testing.T) {
	for _, senderID := range []int64{111, 999} {
		ts := newTestServer(t, "secret")

		body := fmt.Sprintf(`{"update_id":1,"message":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"},"from":{"id":%d,"is_bot":false},"text":"hello bot"}}`, senderID)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", strings.NewReader(body))
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
		rec := httptest.NewRecorder()

		ts.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("sender %d: status = %d, want 200", senderID, rec.Code)
		}
		if len(ts.tg.Sent) != 0 {
			t.Errorf("sender %d: expected no message sent for a plain-text update", senderID)
		}
	}
}
