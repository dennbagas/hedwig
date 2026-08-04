package notify

import (
	"context"
	"errors"
	"hedwig/internal/telegrambot/telegrambottest"
	"strings"
	"testing"

	"github.com/google/go-github/v88/github"
	"github.com/rs/zerolog"
)

type stubHandler struct {
	called bool
	err    error
}

func (s *stubHandler) Handle(_ context.Context, _ any) error {
	s.called = true
	return s.err
}

func TestDispatcherRoutesToRegisteredHandler(t *testing.T) {
	d := newDispatcher(zerolog.Nop())
	h := &stubHandler{}
	d.Register("push", h)

	if err := d.Dispatch(context.Background(), "push", "some-event"); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if !h.called {
		t.Error("expected the registered handler to be called")
	}
}

func TestDispatcherUnknownEventTypeIsNoop(t *testing.T) {
	d := newDispatcher(zerolog.Nop())
	h := &stubHandler{}
	d.Register("push", h)

	if err := d.Dispatch(context.Background(), "unknown_type", "x"); err != nil {
		t.Fatalf("Dispatch() error = %v, want nil for an unregistered event type", err)
	}
	if h.called {
		t.Error("expected no handler to be called for an unregistered event type")
	}
}

func TestDispatcherWrapsHandlerError(t *testing.T) {
	d := newDispatcher(zerolog.Nop())
	d.Register("push", &stubHandler{err: errors.New("boom")})

	err := d.Dispatch(context.Background(), "push", "x")
	if err == nil {
		t.Fatal("Dispatch() error = nil, want the handler's error wrapped")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "push") {
		t.Errorf("Dispatch() error = %q, want it to mention the event type and wrap the original error", err.Error())
	}
}

// allEventTypesLoader returns a loader with a pass-through template for every
// event type registerAll wires up. Used to verify handler routing without
// caring about message content.
func allEventTypesLoader(t *testing.T) *templateLoader {
	t.Helper()
	l, err := newTemplateLoaderFromStrings(map[string]string{
		"push":                        `push`,
		"pull_request":                `pull_request`,
		"create":                      `create`,
		"issue_comment":               `issue_comment`,
		"pull_request_review":         `pull_request_review`,
		"pull_request_review_comment": `pull_request_review_comment`,
		"workflow_run":                `workflow_run`,
	})
	if err != nil {
		t.Fatalf("newTemplateLoaderFromStrings() error = %v", err)
	}
	return l
}

// TestRegisterAllWiresEveryEventType dispatches one event of each type
// registerAll is supposed to wire up, through the real Dispatcher, and
// confirms each one reaches a handler. Per-handler behavior is covered by
// events_test.go/workflow_run_test.go; this test exists to catch a
// forgotten/mistyped Register call that those wouldn't.
func TestRegisterAllWiresEveryEventType(t *testing.T) {
	tg := telegrambottest.New()
	d := newDispatcher(zerolog.Nop())
	// retryH is nil: every event used below takes the workflow_run
	// "requested" branch, which never touches it.
	registerAll(d, destinations{tg: tg, chatID: 1}, nil, allEventTypesLoader(t))

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
