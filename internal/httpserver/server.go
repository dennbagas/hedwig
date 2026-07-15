package httpserver

import (
	"net/http"

	"github.com/btse/hedwig/internal/githubapp"
	"github.com/btse/hedwig/internal/notify"
	"github.com/btse/hedwig/internal/retry"
	"github.com/btse/hedwig/internal/storage"
	"go.uber.org/zap"
)

type Server struct {
	github         githubapp.Client
	store          storage.Repository
	notifyD        *notify.Dispatcher
	retryH         *retry.Handler
	allowedUserIDs []int64
	telegramSecret string
	logger         *zap.Logger
	mux            *http.ServeMux
}

func New(
	gh githubapp.Client,
	store storage.Repository,
	notifyD *notify.Dispatcher,
	retryH *retry.Handler,
	allowedUserIDs []int64,
	telegramSecret string,
	healthzPath string,
	telegramWebhookPath string,
	logger *zap.Logger,
) *Server {
	s := &Server{
		github:         gh,
		store:          store,
		notifyD:        notifyD,
		retryH:         retryH,
		allowedUserIDs: allowedUserIDs,
		telegramSecret: telegramSecret,
		logger:         logger,
		mux:            http.NewServeMux(),
	}

	s.mux.HandleFunc("/webhooks/github", s.handleGitHubWebhook)
	s.mux.HandleFunc(telegramWebhookPath, s.handleTelegramWebhook)
	s.mux.HandleFunc(healthzPath, s.handleHealthz)

	return s
}

func (s *Server) Handler() http.Handler {
	return chain(s.mux,
		requestIDMiddleware(s.logger),
		loggingMiddleware(),
	)
}
