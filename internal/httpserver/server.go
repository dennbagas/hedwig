package httpserver

import (
	"net/http"

	"hedwig/internal/database"
	"hedwig/internal/githubapp"
	"hedwig/internal/notify"
	"hedwig/internal/retry"

	"github.com/rs/zerolog"
)

type Server struct {
	github         githubapp.Client
	store          database.Repository
	notifyD        *notify.Dispatcher
	retryH         *retry.Handler
	telegramSecret string
	healthzPath    string
	logger         zerolog.Logger
	mux            *http.ServeMux
}

func New(
	gh githubapp.Client,
	store database.Repository,
	notifyD *notify.Dispatcher,
	retryH *retry.Handler,
	telegramSecret string,
	healthzPath string,
	telegramWebhookPath string,
	logger zerolog.Logger,
) *Server {
	s := &Server{
		github:         gh,
		store:          store,
		notifyD:        notifyD,
		retryH:         retryH,
		telegramSecret: telegramSecret,
		healthzPath:    healthzPath,
		logger:         logger,
		mux:            http.NewServeMux(),
	}

	s.mux.HandleFunc("/webhooks/github", s.handleGitHubWebhook)
	s.mux.HandleFunc(telegramWebhookPath, s.handleTelegramWebhook)
	s.mux.HandleFunc(healthzPath, s.handleHealthz)

	return s
}

func (s *Server) Handler() http.Handler {
	skipRequestLogForPath := []string{s.healthzPath}
	return chain(s.mux,
		requestIDMiddleware(s.logger),
		loggingMiddleware(skipRequestLogForPath...),
	)
}
