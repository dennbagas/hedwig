package notify

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btse/hedwig/internal/githubapp/githubapptest"
	"github.com/btse/hedwig/internal/retry"
	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot/telegrambottest"
	"github.com/google/go-github/v66/github"
	"go.uber.org/zap"
)

// newTestRetryHandler builds a real *retry.Handler backed by fakes and a
// real temp-file SQLite repository, so workflowRunHandler's delegation to
// the retry package can be exercised end-to-end rather than mocked away.
func newTestRetryHandler(t *testing.T) (*retry.Handler, *telegrambottest.FakeClient, *githubapptest.FakeClient) {
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
	store := storage.NewSQLiteRepository(db)

	tg := telegrambottest.New()
	gh := githubapptest.New()
	return retry.New(store, tg, gh, zap.NewNop()), tg, gh
}

func TestWorkflowRunHandlerRequested(t *testing.T) {
	tg := telegrambottest.New()
	h := &workflowRunHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.WorkflowRunEvent](t, `{
		"action": "requested",
		"workflow_run": {"name": "CI", "head_branch": "main", "html_url": "https://github.com/acme/widgets/actions/runs/1"},
		"repository": {"full_name": "acme/widgets"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 1 {
		t.Fatalf("len(tg.Sent) = %d, want 1", len(tg.Sent))
	}
	if !strings.Contains(tg.Sent[0].Text, "CI/CD started") {
		t.Errorf("text = %q, want it to say CI/CD started", tg.Sent[0].Text)
	}
}

func TestWorkflowRunHandlerCompletedSuccess(t *testing.T) {
	tg := telegrambottest.New()
	h := &workflowRunHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.WorkflowRunEvent](t, `{
		"action": "completed",
		"workflow_run": {"name": "CI", "conclusion": "success", "html_url": "https://github.com/acme/widgets/actions/runs/1"},
		"repository": {"full_name": "acme/widgets"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !strings.Contains(tg.Sent[0].Text, "CI/CD success") {
		t.Errorf("text = %q, want it to say CI/CD success", tg.Sent[0].Text)
	}
}

func TestWorkflowRunHandlerCompletedFailureDelegatesToRetry(t *testing.T) {
	retryH, tg, _ := newTestRetryHandler(t)
	h := &workflowRunHandler{tg: tg, chatID: 1, retryH: retryH}
	event := unmarshalEvent[github.WorkflowRunEvent](t, `{
		"action": "completed",
		"workflow_run": {"id": 55, "name": "CI", "conclusion": "failure", "html_url": "https://github.com/acme/widgets/actions/runs/55"},
		"repository": {"full_name": "acme/widgets", "name": "widgets", "owner": {"login": "acme"}}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	// retry.Handler.NotifyFailure sends the initial message, then edits it
	// to attach the retry button.
	if len(tg.Sent) != 2 {
		t.Fatalf("len(tg.Sent) = %d, want 2 (send + button edit)", len(tg.Sent))
	}
	if !strings.Contains(tg.Sent[0].Text, "CI/CD failed") {
		t.Errorf("text = %q, want it to say CI/CD failed", tg.Sent[0].Text)
	}
	if len(tg.Sent[1].Params.Keyboard) != 1 {
		t.Errorf("expected the edited message to carry the retry button")
	}
}

func TestWorkflowRunHandlerWrongEventType(t *testing.T) {
	tg := telegrambottest.New()
	h := &workflowRunHandler{tg: tg, chatID: 1}
	if err := h.Handle(context.Background(), &github.PushEvent{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a mismatched event type")
	}
}
