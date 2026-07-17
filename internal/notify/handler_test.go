package notify

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	d := NewDispatcher(nil, 0, zerolog.Nop())
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
	d := NewDispatcher(nil, 0, zerolog.Nop())
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
	d := NewDispatcher(nil, 0, zerolog.Nop())
	d.Register("push", &stubHandler{err: errors.New("boom")})

	err := d.Dispatch(context.Background(), "push", "x")
	if err == nil {
		t.Fatal("Dispatch() error = nil, want the handler's error wrapped")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "push") {
		t.Errorf("Dispatch() error = %q, want it to mention the event type and wrap the original error", err.Error())
	}
}
