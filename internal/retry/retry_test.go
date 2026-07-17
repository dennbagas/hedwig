package retry

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hedwig/internal/database"
	"hedwig/internal/githubapp/githubapptest"
	"hedwig/internal/telegrambot"
	"hedwig/internal/telegrambot/telegrambottest"

	"github.com/rs/zerolog"
)

// newTestRepo returns a real SQLite-backed repository (temp file, cleaned up
// automatically) plus the underlying *sql.DB for tests that need to
// manipulate rows directly (e.g. backdating timestamps for expiry tests).
func newTestRepo(t *testing.T) (*sql.DB, database.Repository) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("database.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close() error = %v", err)
		}
	})
	return db, database.NewSQLiteRepository(db)
}

func newTestHandler(t *testing.T) (*Handler, *telegrambottest.FakeClient, *githubapptest.FakeClient, database.Repository) {
	t.Helper()
	_, store := newTestRepo(t)
	tg := telegrambottest.New()
	gh := githubapptest.New()
	return New(store, tg, gh, zerolog.Nop()), tg, gh, store
}

const testFailureText = `CI/CD failed: <b>Build</b>
acme/widgets
<a href="https://github.com/acme/widgets/actions/runs/55">View run</a>`

func TestNotifyFailureSuccess(t *testing.T) {
	h, tg, _, store := newTestHandler(t)
	ctx := context.Background()

	err := h.NotifyFailure(ctx, 100, "Build", "acme", "widgets", 55, testFailureText)
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
	if sendMsg.Text != testFailureText {
		t.Errorf("sent text = %q, want %q", sendMsg.Text, testFailureText)
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
	if rec.ChatID != 100 || rec.RunID != 55 || rec.Repo != "acme/widgets" || rec.Status != database.RetryStatusPending {
		t.Errorf("persisted retry = %+v, want matching fields", rec)
	}
	if rec.MessageText != testFailureText {
		t.Errorf("persisted MessageText = %q, want %q", rec.MessageText, testFailureText)
	}
}

func TestNotifyFailureSendMessageError(t *testing.T) {
	h, tg, _, store := newTestHandler(t)
	tg.SendMessageErr = errors.New("telegram is down")

	err := h.NotifyFailure(context.Background(), 1, "Build", "acme", "widgets", 1, testFailureText)
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

	err := h.NotifyFailure(context.Background(), 1, "Build", "acme", "widgets", 1, testFailureText)
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
	if rec.Status != database.RetryStatusPending {
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

	id, err := store.CreateRetry(ctx, database.CICDRetry{ChatID: 1, MessageID: 2, RunID: 3, Repo: "a/b", Status: database.RetryStatusExpired})
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

	id, err := store.CreateRetry(ctx, database.CICDRetry{ChatID: 1, MessageID: 2, RunID: 55, Repo: "acme/widgets", Status: database.RetryStatusPending})
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
	if rec.Status != database.RetryStatusPending {
		t.Errorf("status = %q, want to remain pending after a failed retry attempt", rec.Status)
	}
}

func TestHandleCallbackRerunErrorEscapesHTML(t *testing.T) {
	h, tg, gh, store := newTestHandler(t)
	ctx := context.Background()

	id, err := store.CreateRetry(ctx, database.CICDRetry{ChatID: 1, MessageID: 2, RunID: 1, Repo: "acme/widgets", Status: database.RetryStatusPending})
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

	id, err := store.CreateRetry(ctx, database.CICDRetry{
		ChatID: 1, MessageID: 2, RunID: 55, Repo: "acme/widgets",
		MessageText: testFailureText,
		Status:      database.RetryStatusPending,
	})
	if err != nil {
		t.Fatalf("CreateRetry() error = %v", err)
	}

	if err := h.HandleCallback(ctx, "cbq-1", 1, 2, id); err != nil {
		t.Fatalf("HandleCallback() error = %v", err)
	}

	if len(gh.RerunCalls) != 1 {
		t.Fatalf("RerunCalls = %+v, want exactly one call", gh.RerunCalls)
	}
	if len(tg.Sent) != 1 || !tg.Sent[0].Edited {
		t.Fatalf("tg.Sent = %+v, want exactly one edited message", tg.Sent)
	}
	wantText := testFailureText + "\n\n✅ Retry request sent"
	if tg.Sent[0].Text != wantText {
		t.Errorf("text = %q, want %q", tg.Sent[0].Text, wantText)
	}

	rec, err := store.GetRetry(ctx, id)
	if err != nil {
		t.Fatalf("GetRetry() error = %v", err)
	}
	if rec.Status != database.RetryStatusRetried {
		t.Errorf("status = %q, want %q", rec.Status, database.RetryStatusRetried)
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
