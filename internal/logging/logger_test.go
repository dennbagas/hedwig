package logging

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNew(t *testing.T) {
	logger, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if logger == nil {
		t.Fatal("New() returned a nil logger")
	}
}

func TestWithContextFromContextRoundTrip(t *testing.T) {
	core, _ := observer.New(zapcore.InfoLevel)
	want := zap.New(core)

	ctx := WithContext(context.Background(), want)
	got := FromContext(ctx)

	if got != want {
		t.Error("FromContext() did not return the logger stored via WithContext()")
	}
}

func TestFromContextFallsBackOnBareContext(t *testing.T) {
	logger := FromContext(context.Background())
	if logger == nil {
		t.Fatal("FromContext() on a bare context returned nil, want a working default logger")
	}
	// Should not panic when used.
	logger.Info("smoke test")
}

func TestFromContextFallsBackWhenValueIsWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey{}, "not a logger")
	logger := FromContext(ctx)
	if logger == nil {
		t.Fatal("FromContext() with a wrong-typed value returned nil, want a working default logger")
	}
}

func TestFieldRequestID(t *testing.T) {
	field := FieldRequestID("abc123")
	if field.Key != "request_id" {
		t.Errorf("field.Key = %q, want %q", field.Key, "request_id")
	}
	if field.String != "abc123" {
		t.Errorf("field.String = %q, want %q", field.String, "abc123")
	}
}
