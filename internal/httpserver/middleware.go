package httpserver

import (
	"net/http"
	"time"

	"hedwig/internal/logging"
	"hedwig/internal/telegrambot"

	"github.com/rs/zerolog"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func requestIDMiddleware(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := telegrambot.GenerateRequestID()
			w.Header().Set("X-Request-ID", id)
			reqLogger := logger.With().Str("request_id", id).Logger()
			ctx := logging.WithContext(r.Context(), reqLogger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func loggingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			l := logging.FromContext(r.Context())
			l.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", rec.status).
				Dur("duration", time.Since(start)).
				Msg("request")
		})
	}
}

func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
