package prcreate

import (
	"github.com/btse/hedwig/internal/config"
	"github.com/btse/hedwig/internal/githubapp"
	"github.com/btse/hedwig/internal/storage"
	"github.com/btse/hedwig/internal/telegrambot"
	"go.uber.org/zap"
)

// Handler manages the multi-step PR creation flow.
type Handler struct {
	store        storage.Repository
	tg           telegrambot.Client
	github       githubapp.Client
	repos        []config.RepoConfig
	sourceBranch string
	targetBranch string
	logger       *zap.Logger
}

func New(
	store storage.Repository,
	tg telegrambot.Client,
	gh githubapp.Client,
	repos []config.RepoConfig,
	sourceBranch, targetBranch string,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		store:        store,
		tg:           tg,
		github:       gh,
		repos:        repos,
		sourceBranch: sourceBranch,
		targetBranch: targetBranch,
		logger:       logger,
	}
}
