package githubapp

import (
	"context"
	"net/http"
)

type Client interface {
	ValidateWebhook(r *http.Request) (payload []byte, err error)
	ParseWebhook(eventType string, payload []byte) (any, error)
	RerunFailedJobs(ctx context.Context, owner, repo string, runID int64) error
	// CreatePR returns the HTML URL of the newly created pull request.
	CreatePR(ctx context.Context, owner, repo, title, body, head, base string) (string, error)
}
