package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
)

func TestNew(t *testing.T) {
	logger := New("info")
	if logger.GetLevel() == zerolog.Disabled {
		t.Fatal("New() returned a disabled logger")
	}
}

func TestNewDefaultsToInfoOnInvalidLevel(t *testing.T) {
	logger := New("bogus")
	if logger.GetLevel() != zerolog.InfoLevel {
		t.Errorf("New(%q) level = %v, want info", "bogus", logger.GetLevel())
	}
}

func TestNewRespectsConfiguredLevel(t *testing.T) {
	logger := New("debug")
	if logger.GetLevel() != zerolog.DebugLevel {
		t.Errorf("New(debug) level = %v, want debug", logger.GetLevel())
	}
}

func TestWithContextFromContextRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := zerolog.New(&buf)

	ctx := WithContext(context.Background(), want)
	got := FromContext(ctx)

	got.Info().Msg("ping")
	if buf.Len() == 0 {
		t.Error("FromContext() did not return the logger stored via WithContext()")
	}
}

func TestFromContextFallsBackOnBareContext(t *testing.T) {
	logger := FromContext(context.Background())
	if logger.GetLevel() == zerolog.Disabled {
		t.Fatal("FromContext() on a bare context returned a disabled logger")
	}
}

func TestNewOutputsTimestampFirst(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).With().Timestamp().Logger().Hook(errorStackHook{})

	logger.Info().Msg("test")

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := raw["timestamp"]; !ok {
		t.Error("expected 'timestamp' key in log output")
	}
}

func TestErrorStackHookAddsStacktrace(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Hook(errorStackHook{})

	logger.Error().Msg("boom")

	var entry map[string]string
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if entry["stacktrace"] == "" {
		t.Error("expected 'stacktrace' field on error-level log")
	}
}

func TestErrorStackHookAbsentOnInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Hook(errorStackHook{})

	logger.Info().Msg("ok")

	var entry map[string]string
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := entry["stacktrace"]; ok {
		t.Error("expected no 'stacktrace' field on info-level log")
	}
}
