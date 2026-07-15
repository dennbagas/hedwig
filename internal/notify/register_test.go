package notify

import (
	"context"
	"testing"

	"hedwig/internal/telegrambot/telegrambottest"
	"github.com/google/go-github/v88/github"
	"go.uber.org/zap"
)

// TestRegisterAllWiresEveryEventType dispatches one event of each type
// RegisterAll is supposed to wire up, through the real Dispatcher, and
// confirms each one reaches a handler. Per-handler behavior is covered by
// events_test.go/workflow_run_test.go; this test exists to catch a
// forgotten/mistyped Register call that those wouldn't.
func TestRegisterAllWiresEveryEventType(t *testing.T) {
	tg := telegrambottest.New()
	d := NewDispatcher(tg, 1, zap.NewNop())
	// retryH is nil: every event used below takes the workflow_run
	// "requested" branch, which never touches it.
	RegisterAll(d, tg, nil, 1)

	tests := []struct {
		eventType string
		event     any
	}{
		{"push", unmarshalEvent[github.PushEvent](t, `{"pusher":{"name":"a"},"repository":{"full_name":"a/b"}}`)},
		{"pull_request", unmarshalEvent[github.PullRequestEvent](t, `{"action":"opened","pull_request":{"title":"t","html_url":"u"}}`)},
		{"create", unmarshalEvent[github.CreateEvent](t, `{"ref_type":"tag","ref":"v1","repository":{"full_name":"a/b"}}`)},
		{"issue_comment", unmarshalEvent[github.IssueCommentEvent](t, `{"action":"created","issue":{"title":"t","pull_request":{"url":"x"}},"comment":{}}`)},
		{"pull_request_review", unmarshalEvent[github.PullRequestReviewEvent](t, `{"action":"submitted","review":{},"pull_request":{}}`)},
		{"pull_request_review_comment", unmarshalEvent[github.PullRequestReviewCommentEvent](t, `{"action":"created","pull_request":{},"comment":{}}`)},
		{"workflow_run", unmarshalEvent[github.WorkflowRunEvent](t, `{"action":"requested","workflow_run":{"name":"CI"},"repository":{"full_name":"a/b"}}`)},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			before := len(tg.Sent)
			if err := d.Dispatch(context.Background(), tt.eventType, tt.event); err != nil {
				t.Fatalf("Dispatch(%q) error = %v", tt.eventType, err)
			}
			if len(tg.Sent) != before+1 {
				t.Errorf("Dispatch(%q): len(tg.Sent) = %d, want %d (a registered handler should have sent one message)",
					tt.eventType, len(tg.Sent), before+1)
			}
		})
	}
}
