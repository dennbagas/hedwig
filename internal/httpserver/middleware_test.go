package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"hedwig/internal/logging"
)

func TestRequestIDMiddlewareSetsHeaderAndContext(t *testing.T) {
	var loggerInContext *zap.Logger
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loggerInContext = logging.FromContext(r.Context())
	})

	handler := requestIDMiddleware(zap.NewNop())(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	id := rec.Header().Get("X-Request-ID")
	if id == "" {
		t.Error("expected X-Request-ID header to be set")
	}
	if loggerInContext == nil {
		t.Error("expected a logger to be attached to the request context")
	}
}

func TestRequestIDMiddlewareUsesTheConfiguredLogger(t *testing.T) {
	// Regression test: requestIDMiddleware must derive the per-request
	// logger from the logger it was constructed with, not fall back to
	// logging.FromContext's default (a fresh zap.NewProduction() instance)
	// just because the incoming request has no logger attached yet.
	core, logs := observer.New(zapcore.InfoLevel)
	configured := zap.New(core)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logging.FromContext(r.Context()).Info("from handler")
	})

	handler := requestIDMiddleware(configured)(next)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	entries := logs.All()
	if len(entries) != 1 || entries[0].Message != "from handler" {
		t.Fatalf("logs = %+v, want exactly one entry logged through the configured logger", entries)
	}
}

func TestRequestIDMiddlewareGeneratesUniqueIDs(t *testing.T) {
	seen := make(map[string]bool)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := requestIDMiddleware(zap.NewNop())(next)

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
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

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

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("logged entries = %d, want 1", len(entries))
	}
	ctxMap := entries[0].ContextMap()
	status, ok := ctxMap["status"].(int64)
	if !ok || int(status) != http.StatusTeapot {
		t.Errorf("logged status = %v, want %d", ctxMap["status"], http.StatusTeapot)
	}
	if method, _ := ctxMap["method"].(string); method != http.MethodGet {
		t.Errorf("logged method = %v, want %q", ctxMap["method"], http.MethodGet)
	}
	if path, _ := ctxMap["path"].(string); path != "/foo" {
		t.Errorf("logged path = %v, want %q", ctxMap["path"], "/foo")
	}
}

func TestLoggingMiddlewareDefaultsToOKWhenHandlerNeverWritesStatus(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hi")) // implicit 200, never calls WriteHeader directly
	})
	handler := loggingMiddleware()(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(logging.WithContext(req.Context(), logger))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	status, _ := logs.All()[0].ContextMap()["status"].(int64)
	if int(status) != http.StatusOK {
		t.Errorf("logged status = %d, want %d", status, http.StatusOK)
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
