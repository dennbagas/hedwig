package httpserver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"hedwig/internal/database"
)

// signSlackRequest returns the timestamp/signature header values Slack
// would attach to a request with this body, using testSlackSigningSecret —
// mirroring the exact v0 scheme verifySlackSignature checks.
func signSlackRequest(t *testing.T, timestamp string, body string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(testSlackSigningSecret))
	mac.Write([]byte("v0:" + timestamp + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func blockActionsBody(t *testing.T, value string) string {
	t.Helper()
	payload := fmt.Sprintf(`{"type":"block_actions","actions":[{"value":%q}],"channel":{"id":"C1"},"message":{"ts":"1111.2222"}}`, value)
	return "payload=" + url.QueryEscape(payload)
}

func newSlackRequest(t *testing.T, body string, timestamp string, validSignature bool) *http.Request {
	t.Helper()
	sig := signSlackRequest(t, timestamp, body)
	if !validSignature {
		sig = "v0=0000000000000000000000000000000000000000000000000000000000000000"
	}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/slack/interactions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", sig)
	return req
}

func TestSlackWebhookRetryCallbackRoutesToRetryHandler(t *testing.T) {
	ts := newTestServer(t, "secret")
	ctx := context.Background()

	id, err := ts.store.CreateRetry(ctx, database.CICDRetry{RunID: 55, Repo: "acme/widgets", Status: database.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	if err := ts.store.CreateRetryTarget(ctx, database.RetryTarget{RetryID: id, Platform: database.PlatformSlack, ChatRef: "C1", MessageRef: "1111.2222", MessageText: "msg"}); err != nil {
		t.Fatalf("CreateRetryTarget() error = %v", err)
	}

	value := fmt.Sprintf("hedwig:retry:trigger:%d", id)
	body := blockActionsBody(t, value)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := newSlackRequest(t, body, timestamp, true)
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(ts.gh.RerunCalls) != 1 {
		t.Fatalf("RerunCalls = %+v, want exactly one (retry.Handler should have run)", ts.gh.RerunCalls)
	}
	if len(ts.slack.Sent) != 1 || !ts.slack.Sent[0].Updated {
		t.Fatalf("slack.Sent = %+v, want the retry message updated", ts.slack.Sent)
	}
}

func TestSlackWebhookInvalidSignature(t *testing.T) {
	ts := newTestServer(t, "secret")

	body := blockActionsBody(t, "hedwig:retry:trigger:1")
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := newSlackRequest(t, body, timestamp, false)
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if len(ts.gh.RerunCalls) != 0 {
		t.Error("expected no GitHub calls for an invalid signature")
	}
}

func TestSlackWebhookStaleTimestampRejected(t *testing.T) {
	ts := newTestServer(t, "secret")

	body := blockActionsBody(t, "hedwig:retry:trigger:1")
	staleTimestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	req := newSlackRequest(t, body, staleTimestamp, true)
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (replay-window rejection)", rec.Code)
	}
}

func TestSlackWebhookMalformedPayloadIsNotFatal(t *testing.T) {
	ts := newTestServer(t, "secret")

	body := "payload=" + url.QueryEscape("not valid json")
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := newSlackRequest(t, body, timestamp, true)
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (malformed payload is swallowed, not fatal)", rec.Code)
	}
}

func TestSlackWebhookUnknownFeature(t *testing.T) {
	ts := newTestServer(t, "secret")

	body := blockActionsBody(t, "hedwig:mystery-feature:action:1")
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := newSlackRequest(t, body, timestamp, true)
	rec := httptest.NewRecorder()

	ts.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(ts.gh.RerunCalls) != 0 {
		t.Error("expected no GitHub calls for an unknown callback feature")
	}
}
