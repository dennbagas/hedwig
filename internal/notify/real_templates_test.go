package notify

import (
	"context"
	"strings"
	"testing"

	"hedwig/internal/slackbot/slackbottest"
	"hedwig/internal/telegrambot/telegrambottest"

	"github.com/rs/zerolog"

	"github.com/google/go-github/v88/github"
)

// realTemplatesLoader loads the actual templates/ directory shipped with the
// repo, not an inline test fixture — regressions like a template referencing
// a context field the Go struct doesn't have (e.g. the missing
// PushContext.Deleted field) only surface here, not against hand-written
// test templates.
func realTemplatesLoader(t *testing.T) *templateLoader {
	t.Helper()
	l, err := newTemplateLoader("../../templates", zerolog.Nop())
	if err != nil {
		t.Fatalf("newTemplateLoader(real templates dir) error = %v", err)
	}
	return l
}

func TestRealTemplatesLoadWithoutError(t *testing.T) {
	realTemplatesLoader(t) // fails the test via t.Fatalf if loading/parsing errors
}

func TestRealTemplatesPushHandler(t *testing.T) {
	loader := realTemplatesLoader(t)

	tests := []struct {
		name       string
		payload    string
		wantEmpty  bool
		wantInText []string
	}{
		{
			name:       "branch push",
			payload:    `{"ref":"refs/heads/main","pusher":{"name":"alice"},"repository":{"full_name":"acme/widgets"},"head_commit":{"message":"Fix bug"}}`,
			wantInText: []string{"acme/widgets", "main", "alice"},
		},
		{
			name:      "deleted branch push",
			payload:   `{"ref":"refs/heads/feature-x","deleted":true,"pusher":{"name":"alice"},"repository":{"full_name":"acme/widgets"}}`,
			wantEmpty: true,
		},
		{
			name:      "tag push",
			payload:   `{"ref":"refs/tags/v1.0.0","pusher":{"name":"alice"},"repository":{"full_name":"acme/widgets"}}`,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := telegrambottest.New()
			slack := slackbottest.New()
			h := &pushHandler{destinations: destinations{tg: tg, chatID: 1, slack: slack, slackChanID: "C1"}, loader: loader}
			event := unmarshalEvent[github.PushEvent](t, tt.payload)

			if err := h.Handle(context.Background(), event); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}

			if tt.wantEmpty {
				if len(tg.Sent) != 0 || len(slack.Sent) != 0 {
					t.Errorf("tg.Sent=%+v slack.Sent=%+v, want no notifications", tg.Sent, slack.Sent)
				}
				return
			}
			if len(tg.Sent) != 1 || len(slack.Sent) != 1 {
				t.Fatalf("tg.Sent=%+v slack.Sent=%+v, want one message on each platform", tg.Sent, slack.Sent)
			}
			for _, want := range tt.wantInText {
				if !strings.Contains(tg.Sent[0].Text, want) {
					t.Errorf("telegram text = %q, want it to contain %q", tg.Sent[0].Text, want)
				}
				if !strings.Contains(slack.Sent[0].Text, want) {
					t.Errorf("slack text = %q, want it to contain %q", slack.Sent[0].Text, want)
				}
			}
			if !strings.Contains(slack.Sent[0].Text, "> Repository:") {
				t.Errorf("slack text = %q, want the detail block blockquoted", slack.Sent[0].Text)
			}
		})
	}
}

func TestRealTemplatesSlackQuoteFormatting(t *testing.T) {
	loader := realTemplatesLoader(t)

	tests := []struct {
		name      string
		eventType string
		event     any
	}{
		{
			"pull_request opened",
			"pull_request",
			unmarshalEvent[github.PullRequestEvent](t, `{"action":"opened","pull_request":{"title":"t","user":{"login":"bob"},"html_url":"https://x"},"repository":{"full_name":"acme/widgets"}}`),
		},
		{
			"issue_comment on PR",
			"issue_comment",
			unmarshalEvent[github.IssueCommentEvent](t, `{"action":"created","issue":{"title":"t","pull_request":{"url":"x"}},"sender":{"login":"carol"},"comment":{"body":"hi","html_url":"https://x"}}`),
		},
		{
			"pull_request_review submitted",
			"pull_request_review",
			unmarshalEvent[github.PullRequestReviewEvent](t, `{"action":"submitted","review":{"state":"approved","html_url":"https://x"},"pull_request":{"title":"t"}}`),
		},
		{
			"pull_request_review_comment created",
			"pull_request_review_comment",
			unmarshalEvent[github.PullRequestReviewCommentEvent](t, `{"action":"created","pull_request":{"title":"t"},"sender":{"login":"dave"},"comment":{"body":"hi","html_url":"https://x","path":"main.go","line":10}}`),
		},
		{
			// "success" (not "failure") deliberately avoids the retry.Handler
			// delegation path — that requires a wired-up retryH and is
			// already covered by workflow_run_test.go; this test only
			// checks Slack quote formatting on the plain-send path.
			"workflow_run completed success",
			"workflow_run",
			unmarshalEvent[github.WorkflowRunEvent](t, `{"action":"completed","workflow_run":{"name":"Build and Deploy","conclusion":"success","head_branch":"main","html_url":"https://x"},"repository":{"full_name":"acme/widgets"}}`),
		},
		{
			"release published",
			"release",
			unmarshalEvent[github.ReleaseEvent](t, `{"action":"published","release":{"tag_name":"v1","body":"line1\nline2","html_url":"https://x","author":{"login":"eve"}},"repository":{"full_name":"acme/widgets"}}`),
		},
	}

	handlers := map[string]EventHandler{
		"pull_request":                &pullRequestHandler{loader: loader},
		"issue_comment":               &issueCommentHandler{loader: loader},
		"pull_request_review":         &pullRequestReviewHandler{loader: loader},
		"pull_request_review_comment": &pullRequestReviewCommentHandler{loader: loader},
		"workflow_run":                &workflowRunHandler{loader: loader},
		"release":                     &releaseHandler{loader: loader},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slack := slackbottest.New()
			h := handlers[tt.eventType]
			switch v := h.(type) {
			case *pullRequestHandler:
				v.destinations = destinations{slack: slack, slackChanID: "C1"}
			case *issueCommentHandler:
				v.destinations = destinations{slack: slack, slackChanID: "C1"}
			case *pullRequestReviewHandler:
				v.destinations = destinations{slack: slack, slackChanID: "C1"}
			case *pullRequestReviewCommentHandler:
				v.destinations = destinations{slack: slack, slackChanID: "C1"}
			case *workflowRunHandler:
				v.destinations = destinations{slack: slack, slackChanID: "C1"}
			case *releaseHandler:
				v.destinations = destinations{slack: slack, slackChanID: "C1"}
			}

			if err := h.Handle(context.Background(), tt.event); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if len(slack.Sent) != 1 {
				t.Fatalf("slack.Sent = %+v, want exactly one message", slack.Sent)
			}
			text := slack.Sent[0].Text
			if !strings.Contains(text, "> ") {
				t.Errorf("%s: text = %q, want at least one blockquoted line", tt.name, text)
			}
		})
	}
}
