package httpserver

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-github/v88/github"
)

func alwaysValidValidateWebhook(r *http.Request) ([]byte, error) {
	return io.ReadAll(r.Body)
}

func realParseWebhook(eventType string, payload []byte) (any, error) {
	return github.ParseWebHook(eventType, payload)
}

const pushEventBody = `{"pusher":{"name":"alice"},"repository":{"full_name":"acme/widgets"}}`

func TestGitHubWebhookValidSignatureDispatches(t *testing.T) {
	ts := newTestServer(t, "tg-secret")
	ts.gh.ValidateWebhookFunc = alwaysValidValidateWebhook
	ts.gh.ParseWebhookFunc = realParseWebhook

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(pushEventBody))
	req.Header.Set("X-GitHub-Delivery", "d-1")
	req.Header.Set("X-GitHub-Event", "push")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(ts.tg.Sent) != 1 {
		t.Fatalf("len(tg.Sent) = %d, want 1 (push notification dispatched)", len(ts.tg.Sent))
	}
}

func TestGitHubWebhookInvalidSignature(t *testing.T) {
	ts := newTestServer(t, "tg-secret")
	ts.gh.ValidateWebhookFunc = func(r *http.Request) ([]byte, error) {
		return nil, errors.New("bad signature")
	}

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if len(ts.tg.Sent) != 0 {
		t.Error("expected no dispatch for an invalid signature")
	}
}

func TestGitHubWebhookDuplicateDeliverySkipsDispatch(t *testing.T) {
	ts := newTestServer(t, "tg-secret")
	ts.gh.ValidateWebhookFunc = alwaysValidValidateWebhook
	ts.gh.ParseWebhookFunc = realParseWebhook

	send := func() int {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(pushEventBody))
		req.Header.Set("X-GitHub-Delivery", "d-dup")
		req.Header.Set("X-GitHub-Event", "push")
		rec := httptest.NewRecorder()
		ts.Handler().ServeHTTP(rec, req)
		return rec.Code
	}

	if code := send(); code != http.StatusOK {
		t.Fatalf("first delivery status = %d, want 200", code)
	}
	if code := send(); code != http.StatusOK {
		t.Fatalf("duplicate delivery status = %d, want 200", code)
	}

	if len(ts.tg.Sent) != 1 {
		t.Errorf("len(tg.Sent) = %d, want 1 (duplicate should not re-dispatch)", len(ts.tg.Sent))
	}
}

func TestGitHubWebhookNoDeliveryIDSkipsDedup(t *testing.T) {
	ts := newTestServer(t, "tg-secret")
	ts.gh.ValidateWebhookFunc = alwaysValidValidateWebhook
	ts.gh.ParseWebhookFunc = realParseWebhook

	for i := range 2 {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(pushEventBody))
		req.Header.Set("X-GitHub-Event", "push")
		// Deliberately no X-GitHub-Delivery header.
		rec := httptest.NewRecorder()
		ts.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i, rec.Code)
		}
	}

	if len(ts.tg.Sent) != 2 {
		t.Errorf("len(tg.Sent) = %d, want 2 (no delivery id means no dedup)", len(ts.tg.Sent))
	}
}

func TestGitHubWebhookUnparseableEventType(t *testing.T) {
	ts := newTestServer(t, "tg-secret")
	ts.gh.ValidateWebhookFunc = alwaysValidValidateWebhook
	ts.gh.ParseWebhookFunc = realParseWebhook

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader("{}"))
	req.Header.Set("X-GitHub-Event", "not_a_real_event_type")
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unparseable events are swallowed, not failed)", rec.Code)
	}
	if len(ts.tg.Sent) != 0 {
		t.Error("expected no dispatch for an unparseable event type")
	}
}

func TestGitHubWebhookDispatchFailureDeletesDeliveryAndReturnsNon2xx(t *testing.T) {
	ts := newTestServer(t, "tg-secret")
	ts.gh.ValidateWebhookFunc = alwaysValidValidateWebhook
	ts.gh.ParseWebhookFunc = realParseWebhook
	ts.tg.SendMessageErr = errors.New("telegram is down")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(pushEventBody))
	req.Header.Set("X-GitHub-Delivery", "d-fail")
	req.Header.Set("X-GitHub-Event", "push")
	rec := httptest.NewRecorder()
	ts.Handler().ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("status = %d, want a non-2xx status on dispatch failure", rec.Code)
	}

	// Clear the transient failure and redeliver the same delivery ID: since
	// the record should have been deleted on failure, this must NOT be
	// treated as a duplicate, and should be reprocessed successfully.
	ts.tg.SendMessageErr = nil
	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(pushEventBody))
	req2.Header.Set("X-GitHub-Delivery", "d-fail")
	req2.Header.Set("X-GitHub-Event", "push")
	rec2 := httptest.NewRecorder()
	ts.Handler().ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("redelivery status = %d, want 200", rec2.Code)
	}
	if len(ts.tg.Sent) != 1 {
		t.Errorf("len(tg.Sent) = %d, want 1 (the redelivery should have actually dispatched)", len(ts.tg.Sent))
	}
}
