package notify

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"hedwig/internal/database"
	"hedwig/internal/githubapp/githubapptest"
	"hedwig/internal/retry"
	"hedwig/internal/slackbot/slackbottest"
	"hedwig/internal/telegrambot/telegrambottest"

	"github.com/google/go-github/v88/github"
	"github.com/rs/zerolog"
)

// newTestRetryHandler builds a real *retry.Handler backed by fakes and a
// real temp-file SQLite repository, so workflowRunHandler's delegation to
// the retry package can be exercised end-to-end rather than mocked away.
func newTestRetryHandler(t *testing.T) (*retry.Handler, *telegrambottest.FakeClient, *githubapptest.FakeClient) {
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
	store := database.NewSQLiteRepository(db)

	tg := telegrambottest.New()
	gh := githubapptest.New()
	return retry.New(store, tg, nil, "", gh, true, zerolog.Nop()), tg, gh
}

// newTestRetryHandlerBothPlatforms is like newTestRetryHandler but also
// wires a fake Slack client, for tests that verify a CI/CD failure fans out
// to both platforms.
func newTestRetryHandlerBothPlatforms(t *testing.T) (*retry.Handler, *telegrambottest.FakeClient, *slackbottest.FakeClient, *githubapptest.FakeClient) {
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
	store := database.NewSQLiteRepository(db)

	tg := telegrambottest.New()
	slack := slackbottest.New()
	gh := githubapptest.New()
	return retry.New(store, tg, slack, "C1", gh, true, zerolog.Nop()), tg, slack, gh
}

// workflowRunLoader returns a loader with a template that covers all workflow_run actions.
func workflowRunLoader(t *testing.T) *templateLoader {
	t.Helper()
	src := `{{- if eq .Action "requested" -}}CI/CD started: {{.Name}} {{.Repo}} on {{.Branch}}
{{- else if eq .Action "completed" -}}CI/CD {{.Conclusion}}: {{.Name}} {{.Repo}}
{{- end -}}`
	l, err := newTemplateLoaderFromStrings(map[string]string{"workflow_run": src})
	if err != nil {
		t.Fatalf("newTemplateLoaderFromStrings() error = %v", err)
	}
	return l
}

// workflowRunLoaderBothPlatforms is like workflowRunLoader but also
// registers a workflow_run.slack template.
func workflowRunLoaderBothPlatforms(t *testing.T) *templateLoader {
	t.Helper()
	telegramSrc := `{{- if eq .Action "requested" -}}CI/CD started: {{.Name}} {{.Repo}} on {{.Branch}}
{{- else if eq .Action "completed" -}}CI/CD {{.Conclusion}}: {{.Name}} {{.Repo}}
{{- end -}}`
	slackSrc := `{{- if eq .Action "completed" -}}slack CI/CD {{.Conclusion}}: {{.Name}} {{.Repo}}{{- end -}}`
	l, err := newTemplateLoaderFromStrings(map[string]string{
		"workflow_run":       telegramSrc,
		"workflow_run.slack": slackSrc,
	})
	if err != nil {
		t.Fatalf("newTemplateLoaderFromStrings() error = %v", err)
	}
	return l
}

func TestWorkflowRunHandlerRequested(t *testing.T) {
	tg := telegrambottest.New()
	h := &workflowRunHandler{destinations: destinations{tg: tg, chatID: 1}, loader: workflowRunLoader(t)}
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
	h := &workflowRunHandler{destinations: destinations{tg: tg, chatID: 1}, loader: workflowRunLoader(t)}
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
	h := &workflowRunHandler{destinations: destinations{tg: tg, chatID: 1}, retryH: retryH, loader: workflowRunLoader(t)}
	event := unmarshalEvent[github.WorkflowRunEvent](t, `{
		"action": "completed",
		"workflow_run": {"id": 55, "name": "CI", "conclusion": "failure", "html_url": "https://github.com/acme/widgets/actions/runs/55"},
		"repository": {"full_name": "acme/widgets", "name": "widgets", "owner": {"login": "acme"}}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if len(tg.Sent) != 2 {
		t.Fatalf("len(tg.Sent) = %d, want 2 (send + button edit)", len(tg.Sent))
	}
	if !strings.Contains(tg.Sent[0].Text, "CI/CD failure") {
		t.Errorf("text = %q, want it to contain CI/CD failure", tg.Sent[0].Text)
	}
	if len(tg.Sent[1].Params.Keyboard) != 1 {
		t.Errorf("expected the edited message to carry the retry button")
	}
}

func TestWorkflowRunHandlerCompletedFailureRetryDisabled(t *testing.T) {
	tg := telegrambottest.New()
	h := &workflowRunHandler{destinations: destinations{tg: tg, chatID: 1}, retryDisabled: true, loader: workflowRunLoader(t)}
	event := unmarshalEvent[github.WorkflowRunEvent](t, `{
		"action": "completed",
		"workflow_run": {"id": 55, "name": "CI", "conclusion": "failure", "html_url": "https://github.com/acme/widgets/actions/runs/55"},
		"repository": {"full_name": "acme/widgets", "name": "widgets", "owner": {"login": "acme"}}
	}`)

	// retryH is nil, so a nil-pointer call would panic if retryDisabled were
	// not honored — proving the handler takes the plain-send path instead.
	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if len(tg.Sent) != 1 {
		t.Fatalf("len(tg.Sent) = %d, want 1 (plain send, no button edit)", len(tg.Sent))
	}
	if !strings.Contains(tg.Sent[0].Text, "CI/CD failure") {
		t.Errorf("text = %q, want it to contain CI/CD failure", tg.Sent[0].Text)
	}
	if len(tg.Sent[0].Params.Keyboard) != 0 {
		t.Errorf("keyboard = %+v, want no retry button when retry is disabled", tg.Sent[0].Params.Keyboard)
	}
}

func TestWorkflowRunHandlerCompletedFailureDelegatesToRetryBothPlatforms(t *testing.T) {
	retryH, tg, slack, _ := newTestRetryHandlerBothPlatforms(t)
	h := &workflowRunHandler{destinations: destinations{tg: tg, chatID: 1, slack: slack, slackChanID: "C1"}, retryH: retryH, loader: workflowRunLoaderBothPlatforms(t)}
	event := unmarshalEvent[github.WorkflowRunEvent](t, `{
		"action": "completed",
		"workflow_run": {"id": 55, "name": "CI", "conclusion": "failure", "html_url": "https://github.com/acme/widgets/actions/runs/55"},
		"repository": {"full_name": "acme/widgets", "name": "widgets", "owner": {"login": "acme"}}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if len(tg.Sent) != 2 {
		t.Fatalf("len(tg.Sent) = %d, want 2 (send + button edit)", len(tg.Sent))
	}
	if len(slack.Sent) != 1 {
		t.Fatalf("len(slack.Sent) = %d, want 1", len(slack.Sent))
	}
	if !strings.Contains(slack.Sent[0].Text, "slack CI/CD failure") {
		t.Errorf("slack text = %q, want it to contain 'slack CI/CD failure'", slack.Sent[0].Text)
	}
	if len(slack.Sent[0].Buttons) != 1 || slack.Sent[0].Buttons[0].Text != "Retry failed jobs" {
		t.Errorf("slack buttons = %+v, want one Retry failed jobs button", slack.Sent[0].Buttons)
	}
}

func TestWorkflowRunHandlerSkipsWhenNoTemplate(t *testing.T) {
	tg := telegrambottest.New()
	loader, _ := newTemplateLoaderFromStrings(map[string]string{}) // no workflow_run template
	h := &workflowRunHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.WorkflowRunEvent](t, `{
		"action": "requested",
		"workflow_run": {"name": "CI"},
		"repository": {"full_name": "a/b"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message when no template is configured")
	}
}

func TestWorkflowRunHandlerWrongEventType(t *testing.T) {
	tg := telegrambottest.New()
	h := &workflowRunHandler{destinations: destinations{tg: tg, chatID: 1}, loader: workflowRunLoader(t)}
	if err := h.Handle(context.Background(), &github.PushEvent{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a mismatched event type")
	}
}
