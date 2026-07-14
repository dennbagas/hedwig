package notify

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/btse/hedwig/internal/telegrambot/telegrambottest"
	"github.com/google/go-github/v66/github"
)

func unmarshalEvent[T any](t *testing.T, payload string) *T {
	t.Helper()
	var e T
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	return &e
}

func TestPushHandler(t *testing.T) {
	tg := telegrambottest.New()
	h := &pushHandler{tg: tg, chatID: 100}

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

func TestPushHandlerWrongEventType(t *testing.T) {
	tg := telegrambottest.New()
	h := &pushHandler{tg: tg, chatID: 100}
	if err := h.Handle(context.Background(), &github.PullRequestEvent{}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a mismatched event type")
	}
}

func TestPullRequestHandlerOpened(t *testing.T) {
	tg := telegrambottest.New()
	h := &pullRequestHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.PullRequestEvent](t, `{
		"action": "opened",
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
	for _, want := range []string{"PR opened", "bob", "bob:feature", "acme:main"} {
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
	h := &pullRequestHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.PullRequestEvent](t, `{
		"action": "closed",
		"pull_request": {"title": "Add feature", "merged": true, "html_url": "https://github.com/acme/widgets/pull/1"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !strings.Contains(tg.Sent[0].Text, "merged") {
		t.Errorf("text = %q, want it to say merged", tg.Sent[0].Text)
	}
}

func TestPullRequestHandlerClosedNotMerged(t *testing.T) {
	tg := telegrambottest.New()
	h := &pullRequestHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.PullRequestEvent](t, `{
		"action": "closed",
		"pull_request": {"title": "Add feature", "merged": false, "html_url": "https://github.com/acme/widgets/pull/1"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !strings.Contains(tg.Sent[0].Text, "closed without merge") {
		t.Errorf("text = %q, want it to say closed without merge", tg.Sent[0].Text)
	}
}

func TestPullRequestHandlerIgnoresOtherActions(t *testing.T) {
	tg := telegrambottest.New()
	h := &pullRequestHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.PullRequestEvent](t, `{"action": "labeled", "pull_request": {"title": "x"}}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a non opened/closed action")
	}
}

func TestCreateHandlerTag(t *testing.T) {
	tg := telegrambottest.New()
	h := &createHandler{tg: tg, chatID: 1}
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
	for _, want := range []string{"Tag created", "v1.0.0", "acme/widgets", "dana"} {
		if !strings.Contains(text, want) {
			t.Errorf("text = %q, want it to contain %q", text, want)
		}
	}
}

func TestCreateHandlerBranch(t *testing.T) {
	tg := telegrambottest.New()
	h := &createHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.CreateEvent](t, `{
		"ref_type": "branch",
		"ref": "feature/x",
		"repository": {"full_name": "acme/widgets"},
		"sender": {"login": "dana"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if !strings.Contains(tg.Sent[0].Text, "Branch created") {
		t.Errorf("text = %q, want it to say Branch created", tg.Sent[0].Text)
	}
}

func TestCreateHandlerIgnoresOtherRefTypes(t *testing.T) {
	tg := telegrambottest.New()
	h := &createHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.CreateEvent](t, `{"ref_type": "repository", "ref": ""}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a ref_type other than tag/branch")
	}
}

func TestIssueCommentHandlerOnPR(t *testing.T) {
	tg := telegrambottest.New()
	h := &issueCommentHandler{tg: tg, chatID: 1}
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

func TestIssueCommentHandlerIgnoresPlainIssueComments(t *testing.T) {
	tg := telegrambottest.New()
	h := &issueCommentHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.IssueCommentEvent](t, `{
		"action": "created",
		"issue": {"title": "A plain issue"},
		"comment": {"body": "hi"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a comment on a plain issue (no pull_request link)")
	}
}

func TestIssueCommentHandlerIgnoresNonCreatedActions(t *testing.T) {
	tg := telegrambottest.New()
	h := &issueCommentHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.IssueCommentEvent](t, `{
		"action": "edited",
		"issue": {"title": "My PR", "pull_request": {"url": "x"}},
		"comment": {"body": "hi"}
	}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a non-created action")
	}
}

func TestPullRequestReviewHandler(t *testing.T) {
	tg := telegrambottest.New()
	h := &pullRequestReviewHandler{tg: tg, chatID: 1}
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

func TestPullRequestReviewHandlerIgnoresNonSubmitted(t *testing.T) {
	tg := telegrambottest.New()
	h := &pullRequestReviewHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.PullRequestReviewEvent](t, `{"action": "dismissed", "review": {}, "pull_request": {}}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a non-submitted review action")
	}
}

func TestPullRequestReviewCommentHandler(t *testing.T) {
	tg := telegrambottest.New()
	h := &pullRequestReviewCommentHandler{tg: tg, chatID: 1}
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

func TestPullRequestReviewCommentHandlerIgnoresNonCreatedActions(t *testing.T) {
	tg := telegrambottest.New()
	h := &pullRequestReviewCommentHandler{tg: tg, chatID: 1}
	event := unmarshalEvent[github.PullRequestReviewCommentEvent](t, `{"action": "edited", "pull_request": {}, "comment": {}}`)

	if err := h.Handle(context.Background(), event); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(tg.Sent) != 0 {
		t.Error("expected no message for a non-created action")
	}
}
