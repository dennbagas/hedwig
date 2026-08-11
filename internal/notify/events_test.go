package notify

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"hedwig/internal/slackbot/slackbottest"
	"hedwig/internal/telegrambot/telegrambottest"

	"github.com/google/go-github/v88/github"
)

func unmarshalEvent[T any](t *testing.T, payload string) *T {
	t.Helper()
	var e T
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	return &e
}

// mustLoader builds a loader from template strings, failing the test on error.
func mustLoader(t *testing.T, m map[string]string) *templateLoader {
	t.Helper()
	l, err := newTemplateLoaderFromStrings(m)
	if err != nil {
		t.Fatalf("newTemplateLoaderFromStrings() error = %v", err)
	}
	return l
}

func TestPushHandler(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{
		"push": `{{.Repo}} {{.Ref}} {{.Pusher}} {{.Commits}} commit(s) {{.Summary}}`,
	})
	h := &pushHandler{destinations: destinations{tg: tg, chatID: 100}, loader: loader}

	event := unmarshalEvent[github.PushEvent](t, `{
		"ref": "refs/heads/main",
		"pusher": {"name": "alice"},
		"repository": {"full_name": "acme/widgets"},
		"commits": [{}, {}],
		"head_commit": {"message": "Fix <bug> & improve perf"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 1 {
		t.Fatalf("len(tg.Sent) = %d, want 1", len(tg.Sent))
	}
	text := tg.Sent[0].Text
	for _, want := range []string{"acme/widgets", "main", "alice", "2 commit(s)"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, "<bug>") {
		t.Errorf("text = %q, contains unescaped commit message", text)
	}
	if !strings.Contains(text, "&lt;bug&gt;") || !strings.Contains(text, "&amp;") {
		t.Errorf("text = %q, want the commit message HTML-escaped", text)
	}
}

func TestPushHandlerSkipsDeletedBranch(t *testing.T) {
	tg := telegrambottest.New()
	// Mirrors the real templates/push.tmpl filter: branch pushes only,
	// excluding ref deletions (e.g. a branch auto-deleted after a PR merge).
	loader := mustLoader(t, map[string]string{
		"push": `{{- if and (eq .RefType "branch") (not .Deleted) -}}push{{- end -}}`,
	})
	h := &pushHandler{destinations: destinations{tg: tg, chatID: 100}, loader: loader}

	event := unmarshalEvent[github.PushEvent](t, `{
		"ref": "refs/heads/feature-x",
		"deleted": true,
		"pusher": {"name": "alice"},
		"repository": {"full_name": "acme/widgets"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Errorf("tg.Sent = %+v, want none for a deleted-branch push", tg.Sent)
	}
}

func TestPushHandlerBothPlatforms(t *testing.T) {
	tg := telegrambottest.New()
	slack := slackbottest.New()
	loader := mustLoader(t, map[string]string{
		"push":       `telegram: {{.Repo}} {{.Pusher}}`,
		"push.slack": "slack header\n\n{{.Repo}} {{.Pusher}}",
	})
	h := &pushHandler{destinations: destinations{tg: tg, chatID: 100, slack: slack, slackChanID: "C1"}, loader: loader}

	event := unmarshalEvent[github.PushEvent](t, `{
		"ref": "refs/heads/main",
		"pusher": {"name": "alice"},
		"repository": {"full_name": "acme/widgets"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 1 || tg.Sent[0].Text != "telegram: acme/widgets alice" {
		t.Fatalf("tg.Sent = %+v, want one telegram-formatted message", tg.Sent)
	}
	wantSlackText := "slack header\n\n> acme/widgets alice"
	if len(slack.Sent) != 1 || slack.Sent[0].Text != wantSlackText {
		t.Fatalf("slack.Sent = %+v, want one slack-formatted message with quoted detail block (%q)", slack.Sent, wantSlackText)
	}
	if slack.Sent[0].Channel != "C1" {
		t.Errorf("slack channel = %q, want C1", slack.Sent[0].Channel)
	}
}

func TestPushHandlerSlackOnlyWhenTelegramTemplateSkips(t *testing.T) {
	tg := telegrambottest.New()
	slack := slackbottest.New()
	loader := mustLoader(t, map[string]string{
		// Telegram template filters out tag pushes; Slack's doesn't, so
		// only Slack should receive a message for this event.
		"push":       `{{if eq .RefType "branch"}}telegram push{{end}}`,
		"push.slack": `slack push`,
	})
	h := &pushHandler{destinations: destinations{tg: tg, chatID: 100, slack: slack, slackChanID: "C1"}, loader: loader}

	event := unmarshalEvent[github.PushEvent](t, `{
		"ref": "refs/tags/v1.0.0",
		"pusher": {"name": "alice"},
		"repository": {"full_name": "acme/widgets"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Errorf("tg.Sent = %+v, want none (telegram template skipped this event)", tg.Sent)
	}
	if len(slack.Sent) != 1 {
		t.Fatalf("slack.Sent = %+v, want one message", slack.Sent)
	}
}

func TestPushHandlerWrongEventType(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{"push": `ok`})
	h := &pushHandler{destinations: destinations{tg: tg, chatID: 100}, loader: loader}
	if err := h.Handle(context.Background(), &github.PullRequestEvent{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a mismatched event type")
	}
}

func TestHandlerSkipsOnEmptyTemplateOutput(t *testing.T) {
	tg := telegrambottest.New()
	// Template that produces empty output for action "labeled".
	loader := mustLoader(t, map[string]string{
		"pull_request": `{{if eq .Action "opened"}}opened{{end}}`,
	})
	h := &pullRequestHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.PullRequestEvent](t, `{"action":"labeled","pull_request":{"title":"x"}}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Errorf("expected no message when template returns empty, got %d", len(tg.Sent))
	}
}

func TestPullRequestHandlerOpened(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{
		"pull_request": `{{.Action}} {{.Title}} {{.Author}} {{.Head}} {{.Base}} {{.Repo}} {{.URL}}`,
	})
	h := &pullRequestHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.PullRequestEvent](t, `{
		"action": "opened",
		"repository": {"full_name": "acme/widgets"},
		"pull_request": {
			"title": "Add <feature>",
			"user": {"login": "bob"},
			"head": {"label": "bob:feature"},
			"base": {"label": "acme:main"},
			"html_url": "https://github.com/acme/widgets/pull/1"
		}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 1 {
		t.Fatalf("len(tg.Sent) = %d, want 1", len(tg.Sent))
	}
	text := tg.Sent[0].Text
	for _, want := range []string{"opened", "bob", "bob:feature", "acme:main", "acme/widgets"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, "<feature>") {
		t.Errorf("text = %q, contains an unescaped PR title", text)
	}
}

func TestPullRequestHandlerClosedMerged(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{
		"pull_request": `{{.Action}} merged={{.Merged}}`,
	})
	h := &pullRequestHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.PullRequestEvent](t, `{
		"action": "closed",
		"pull_request": {"title": "Add feature", "merged": true, "html_url": "https://github.com/acme/widgets/pull/1"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !strings.Contains(tg.Sent[0].Text, "merged=true") {
		t.Errorf("text = %q, want merged=true", tg.Sent[0].Text)
	}
}

func TestPullRequestHandlerClosedNotMerged(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{
		"pull_request": `{{.Action}} merged={{.Merged}}`,
	})
	h := &pullRequestHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.PullRequestEvent](t, `{
		"action": "closed",
		"pull_request": {"title": "Add feature", "merged": false, "html_url": "https://github.com/acme/widgets/pull/1"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !strings.Contains(tg.Sent[0].Text, "merged=false") {
		t.Errorf("text = %q, want merged=false", tg.Sent[0].Text)
	}
}

func TestCreateHandlerTag(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{
		"create": `{{.RefType}} {{.Ref}} {{.Repo}} {{.Creator}}`,
	})
	h := &createHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.CreateEvent](t, `{
		"ref_type": "tag",
		"ref": "v1.0.0",
		"repository": {"full_name": "acme/widgets"},
		"sender": {"login": "dana"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	text := tg.Sent[0].Text
	for _, want := range []string{"tag", "v1.0.0", "acme/widgets", "dana"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestCreateHandlerBranch(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{
		"create": `{{.RefType}}`,
	})
	h := &createHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.CreateEvent](t, `{
		"ref_type": "branch",
		"ref": "feature/x",
		"repository": {"full_name": "acme/widgets"},
		"sender": {"login": "dana"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !strings.Contains(tg.Sent[0].Text, "branch") {
		t.Errorf("text = %q, want it to contain branch", tg.Sent[0].Text)
	}
}

func TestIssueCommentHandlerOnPR(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{
		"issue_comment": `{{if .IsPR}}{{.PRTitle}} {{.Author}} {{.Body}}{{end}}`,
	})
	h := &issueCommentHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.IssueCommentEvent](t, `{
		"action": "created",
		"issue": {"title": "My PR", "pull_request": {"url": "https://api.github.com/repos/acme/widgets/pulls/1"}},
		"sender": {"login": "carol"},
		"comment": {"body": "Looks <good> to me & thanks", "html_url": "https://github.com/acme/widgets/pull/1#issuecomment-1"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 1 {
		t.Fatalf("len(tg.Sent) = %d, want 1", len(tg.Sent))
	}
	text := tg.Sent[0].Text
	for _, want := range []string{"My PR", "carol"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, "<good>") {
		t.Errorf("text = %q, contains an unescaped comment body", text)
	}
}

func TestIssueCommentHandlerOnPlainIssue(t *testing.T) {
	tg := telegrambottest.New()
	// Template skips when IsPR is false.
	loader := mustLoader(t, map[string]string{
		"issue_comment": `{{if .IsPR}}comment{{end}}`,
	})
	h := &issueCommentHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.IssueCommentEvent](t, `{
		"action": "created",
		"issue": {"title": "A plain issue"},
		"comment": {"body": "hi"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a comment on a plain issue (IsPR=false)")
	}
}

func TestPullRequestReviewHandler(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{
		"pull_request_review": `{{.PRTitle}} {{.Reviewer}} {{.State}}`,
	})
	h := &pullRequestReviewHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.PullRequestReviewEvent](t, `{
		"action": "submitted",
		"review": {"user": {"login": "erin"}, "state": "approved", "html_url": "https://github.com/acme/widgets/pull/1#pullrequestreview-1"},
		"pull_request": {"title": "My PR"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	text := tg.Sent[0].Text
	for _, want := range []string{"My PR", "erin", "Approved"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestPullRequestReviewCommentHandler(t *testing.T) {
	tg := telegrambottest.New()
	loader := mustLoader(t, map[string]string{
		"pull_request_review_comment": `{{.PRTitle}} {{.Author}} {{.File}}:{{.Line}} {{.Body}}`,
	})
	h := &pullRequestReviewCommentHandler{destinations: destinations{tg: tg, chatID: 1}, loader: loader}
	event := unmarshalEvent[github.PullRequestReviewCommentEvent](t, `{
		"action": "created",
		"sender": {"login": "frank"},
		"pull_request": {"title": "My PR"},
		"comment": {"path": "main.go", "line": 42, "body": "consider <renaming> this", "html_url": "https://github.com/acme/widgets/pull/1#discussion_r1"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	text := tg.Sent[0].Text
	for _, want := range []string{"My PR", "frank", "main.go:42"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, "<renaming>") {
		t.Errorf("text = %q, contains an unescaped comment body", text)
	}
}
