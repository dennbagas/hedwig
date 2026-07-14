package retry

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/btse/hedwig/internal/githubapp/githubapptest"
	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
	"github.com/btse/hedwig/internal/telegrambot/telegrambottest"
	"go.uber.org/zap"
)

// newTestRepo returns a real SQLite-backed repository (temp file, cleaned up
// automatically) plus the underlying *sql.DB for tests that need to
// manipulate rows directly (e.g. backdating timestamps for expiry tests).
func newTestRepo(t *testing.T) (*sql.DB, storage.Repository) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close() error = %v", err)
		}
	})
	return db, storage.NewSQLiteRepository(db)
}

func newTestHandler(t *testing.T) (*Handler, *telegrambottest.FakeClient, *githubapptest.FakeClient, storage.Repository) {
	t.Helper()
	_, store := newTestRepo(t)
	tg := telegrambottest.New()
	gh := githubapptest.New()
	return New(store, tg, gh, zap.NewNop()), tg, gh, store
}

func TestNotifyFailureSuccess(t *testing.T) {
	h, tg, _, store := newTestHandler(t)
	ctx := context.Background()

	err := h.NotifyFailure(ctx, 100, "Build", "acme", "widgets", 55, "https://github.com/acme/widgets/actions/runs/55")
	if err != nil {
		t.Fatalf("NotifyFailure() error = %v", err)
	}

	if len(tg.Sent) != 2 {
		t.Fatalf("len(tg.Sent) = %d, want 2 (initial send + button edit)", len(tg.Sent))
	}
	sendMsg := tg.Sent[0]
	if sendMsg.Edited {
		t.Error("first message should be a Send, not an Edit")
	}
	if !strings.Contains(sendMsg.Text, "Build") || !strings.Contains(sendMsg.Text, "acme/widgets") {
		t.Errorf("sent text = %q, want it to mention the workflow name and repo", sendMsg.Text)
	}

	editMsg := tg.Sent[1]
	if !editMsg.Edited {
		t.Error("second message should be an Edit (attaching the button)")
	}
	if len(editMsg.Params.Keyboard) != 1 || len(editMsg.Params.Keyboard[0]) != 1 {
		t.Fatalf("editMsg keyboard = %+v, want one row with one button", editMsg.Params.Keyboard)
	}
	btn := editMsg.Params.Keyboard[0][0]
	if btn.Text != "Retry failed jobs" {
		t.Errorf("button text = %q, want %q", btn.Text, "Retry failed jobs")
	}

	feature, action, payload, err := telegrambot.DecodeCallback(btn.CallbackData)
	if err != nil {
		t.Fatalf("DecodeCallback(%q) error = %v", btn.CallbackData, err)
	}
	if feature != "retry" || action != "trigger" {
		t.Errorf("callback = (%q,%q), want (retry, trigger)", feature, action)
	}
	retryID, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		t.Fatalf("callback payload %q is not an int: %v", payload, err)
	}

	rec, err := store.GetRetry(ctx, retryID)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if rec == nil {
		t.Fatal("expected a retry record to have been persisted")
	}
	if rec.ChatID != 100 || rec.RunID != 55 || rec.Repo != "acme/widgets" || rec.Status != storage.RetryStatusPending {
		t.Errorf("persisted retry = %+v, want matching fields", rec)
	}
}

func TestNotifyFailureEscapesHTML(t *testing.T) {
	h, tg, _, _ := newTestHandler(t)

	err := h.NotifyFailure(context.Background(), 1, "Build <prod>", "acme", "widgets & co", 1, "https://example.com")
	if err != nil {
		t.Fatalf("NotifyFailure() error = %v", err)
	}

	text := tg.Sent[0].Text
	if strings.Contains(text, "<prod>") {
		t.Errorf("text = %q, contains unescaped HTML", text)
	}
	if !strings.Contains(text, "&lt;prod&gt;") {
		t.Errorf("text = %q, want the workflow name HTML-escaped", text)
	}
	if !strings.Contains(text, "widgets &amp; co") {
		t.Errorf("text = %q, want the repo name HTML-escaped", text)
	}
}

func TestNotifyFailureSendMessageError(t *testing.T) {
	h, tg, _, store := newTestHandler(t)
	tg.SendMessageErr = errors.New("telegram is down")

	err := h.NotifyFailure(context.Background(), 1, "Build", "acme", "widgets", 1, "https://example.com")
	if err == nil {
		t.Fatal("NotifyFailure() error = nil, want the send failure to propagate")
	}

	rec, _ := store.GetRetry(context.Background(), 1)
	if rec != nil {
		t.Errorf("expected no retry record to be created when the initial send fails, got %+v", rec)
	}
}

func TestNotifyFailureButtonAttachErrorStillSucceeds(t *testing.T) {
	h, tg, _, store := newTestHandler(t)
	tg.EditMessageErr = errors.New("edit failed")

	err := h.NotifyFailure(context.Background(), 1, "Build", "acme", "widgets", 1, "https://example.com")
	if err != nil {
		t.Fatalf("NotifyFailure() error = %v, want nil even though attaching the button failed", err)
	}

	rec, err := store.GetRetry(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if rec == nil {
		t.Fatal("expected the retry record to exist even though the button never got attached")
	}
	if rec.Status != storage.RetryStatusPending {
		t.Errorf("status = %q, want still pending", rec.Status)
	}
}

func TestHandleCallbackUnknownRetryID(t *testing.T) {
	h, tg, gh, _ := newTestHandler(t)

	if err := h.HandleCallback(context.Background(), "cbq-1", 100, 200, 9999); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 0 {
		t.Errorf("RerunCalls = %+v, want none for an unknown retry ID", gh.RerunCalls)
	}
	if len(tg.Sent) != 1 || !strings.Contains(tg.Sent[0].Text, "no longer valid") {
		t.Errorf("tg.Sent = %+v, want a single 'no longer valid' message", tg.Sent)
	}
	if len(tg.AnsweredCallbacks) != 1 || tg.AnsweredCallbacks[0].CallbackQueryID != "cbq-1" {
		t.Errorf("AnsweredCallbacks = %+v, want the callback query answered", tg.AnsweredCallbacks)
	}
}

func TestHandleCallbackNonPendingStatus(t *testing.T) {
	h, tg, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, storage.CICDRetry{ChatID: 1, MessageID: 2, RunID: 3, Repo: "a/b", Status: storage.RetryStatusExpired})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}

	if err := h.HandleCallback(ctx, "cbq-1", 1, 2, id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 0 {
		t.Errorf("RerunCalls = %+v, want none for an already-expired retry", gh.RerunCalls)
	}
	if !strings.Contains(tg.Sent[0].Text, "no longer valid") {
		t.Errorf("text = %q, want 'no longer valid'", tg.Sent[0].Text)
	}
}

func TestHandleCallbackRerunError(t *testing.T) {
	h, tg, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, storage.CICDRetry{ChatID: 1, MessageID: 2, RunID: 55, Repo: "acme/widgets", Status: storage.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	gh.RerunFailedJobsErr = errors.New("run already active")

	if err := h.HandleCallback(ctx, "cbq-1", 1, 2, id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 1 {
		t.Fatalf("RerunCalls = %+v, want exactly one call", gh.RerunCalls)
	}
	call := gh.RerunCalls[0]
	if call.Owner != "acme" || call.Repo != "widgets" || call.RunID != 55 {
		t.Errorf("RerunCalls[0] = %+v, want acme/widgets run 55", call)
	}

	text := tg.Sent[0].Text
	if !strings.Contains(text, "already active") {
		t.Errorf("text = %q, want it to include the GitHub error", text)
	}
	if !strings.Contains(text, "github.com/acme/widgets/actions/runs/55") {
		t.Errorf("text = %q, want a link back to the run", text)
	}

	rec, err := store.GetRetry(ctx, id)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if rec.Status != storage.RetryStatusPending {
		t.Errorf("status = %q, want to remain pending after a failed retry attempt", rec.Status)
	}
}

func TestHandleCallbackRerunErrorEscapesHTML(t *testing.T) {
	h, tg, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, storage.CICDRetry{ChatID: 1, MessageID: 2, RunID: 1, Repo: "acme/widgets", Status: storage.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}
	gh.RerunFailedJobsErr = errors.New("<script>bad</script>")

	if err := h.HandleCallback(ctx, "cbq-1", 1, 2, id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	text := tg.Sent[0].Text
	if strings.Contains(text, "<script>") {
		t.Errorf("text = %q, contains an unescaped error message", text)
	}
}

func TestHandleCallbackSuccess(t *testing.T) {
	h, tg, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, storage.CICDRetry{ChatID: 1, MessageID: 2, RunID: 55, Repo: "acme/widgets", Status: storage.RetryStatusPending})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}

	if err := h.HandleCallback(ctx, "cbq-1", 1, 2, id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 1 {
		t.Fatalf("RerunCalls = %+v, want exactly one call", gh.RerunCalls)
	}
	if !strings.Contains(tg.Sent[0].Text, "Retrying") {
		t.Errorf("text = %q, want it to mention retrying", tg.Sent[0].Text)
	}

	rec, err := store.GetRetry(ctx, id)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if rec.Status != storage.RetryStatusRetried {
		t.Errorf("status = %q, want %q", rec.Status, storage.RetryStatusRetried)
	}
}

func TestSplitRepo(t *testing.T) {
	tests := []struct {
		in        string
		wantOwner string
		wantRepo  string
	}{
		{"acme/widgets", "acme", "widgets"},
		{"no-slash", "no-slash", ""},
		{"a/b/c", "a", "b/c"},
		{"", "", ""},
	}
	for _, tt := range tests {
		owner, repo := splitRepo(tt.in)
		if owner != tt.wantOwner || repo != tt.wantRepo {
			t.Errorf("splitRepo(%q) = (%q, %q), want (%q, %q)", tt.in, owner, repo, tt.wantOwner, tt.wantRepo)
		}
	}
}
