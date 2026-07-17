package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hedwig/internal/logging"

	"github.com/rs/zerolog"
)

func TestRequestIDMiddlewareSetsHeaderAndContext(t *testing.T) {
	var loggerInContext zerolog.Logger
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loggerInContext = logging.FromContext(r.Context())
	})

	handler := requestIDMiddleware(zerolog.Nop())(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	id := rec.Header().Get("X-Request-ID")
	if id == "" {
		t.Error("expected X-Request-ID header to be set")
	}
	if loggerInContext.GetLevel() == zerolog.Disabled {
		t.Error("expected a logger to be attached to the request context")
	}
}

func TestRequestIDMiddlewareUsesTheConfiguredLogger(t *testing.T) {
	// Regression test: requestIDMiddleware must derive the per-request
	// logger from the logger it was constructed with, not fall back to
	// logging.FromContext's default.
	var buf bytes.Buffer
	configured := zerolog.New(&buf).Level(zerolog.InfoLevel)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := logging.FromContext(r.Context())
		l.Info().Msg("from handler")
	})

	handler := requestIDMiddleware(configured)(next)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(buf.String(), "from handler") {
		t.Fatalf("log output = %q, want it to contain 'from handler' logged through the configured logger", buf.String())
	}
}

func TestRequestIDMiddlewareGeneratesUniqueIDs(t *testing.T) {
	seen := make(map[string]bool)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := requestIDMiddleware(zerolog.Nop())(next)

	for i := range 10 {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		id := rec.Header().Get("X-Request-ID")
		if seen[id] {
			t.Fatalf("duplicate request id %q after %d requests", id, i)
		}
		seen[id] = true
	}
}

func TestLoggingMiddlewareRecordsActualStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.InfoLevel)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := loggingMiddleware()(next)

	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	req = req.WithContext(logging.WithContext(req.Context(), logger))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("response status = %d, want %d (should pass through unchanged)", rec.Code, http.StatusTeapot)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v — output: %s", err, buf.String())
	}
	if status, _ := entry["status"].(float64); int(status) != http.StatusTeapot {
		t.Errorf("logged status = %v, want %d", entry["status"], http.StatusTeapot)
	}
	if method, _ := entry["method"].(string); method != http.MethodGet {
		t.Errorf("logged method = %v, want %q", entry["method"], http.MethodGet)
	}
	if path, _ := entry["path"].(string); path != "/foo" {
		t.Errorf("logged path = %v, want %q", entry["path"], "/foo")
	}
}

func TestLoggingMiddlewareDefaultsToOKWhenHandlerNeverWritesStatus(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.InfoLevel)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hi"))
	})
	handler := loggingMiddleware()(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(logging.WithContext(req.Context(), logger))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v", err)
	}
	if status, _ := entry["status"].(float64); int(status) != http.StatusOK {
		t.Errorf("logged status = %d, want %d", int(status), http.StatusOK)
	}
}

func TestChainAppliesMiddlewaresInDeclaredOrder(t *testing.T) {
	var order []string
	mw := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	})

	h := chain(final, mw("first"), mw("second"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"first", "second", "handler"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want[i])
		}
	}
}
